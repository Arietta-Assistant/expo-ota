package infrastructure

import (
	"expo-open-ota/config"
	"expo-open-ota/internal/bucket"
	"expo-open-ota/internal/dashboard"
	"expo-open-ota/internal/handlers"
	"expo-open-ota/internal/metrics"
	"expo-open-ota/internal/middleware"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func getDashboardPath() string {
	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("Error getting executable path: %v", err)
	}
	exeDir := filepath.Dir(exePath)

	if strings.Contains(exePath, "/var/folders/") || strings.Contains(exePath, "Temp") {
		workingDir, _ := os.Getwd()
		return filepath.Join(workingDir, "dashboard", "dist")
	}
	return filepath.Join(exeDir, "dashboard", "dist")
}

func NewRouter() *gin.Engine {
	router := gin.Default()
	router.Use(middleware.LoggingMiddleware)

	// Health check
	router.GET("/health", handlers.HealthHandler)

	// Metrics
	router.GET("/metrics", func(c *gin.Context) {
		metrics.PrometheusHandler().ServeHTTP(c.Writer, c.Request)
	})

	// Add route for eoas client compatibility with different formats
	// Format 1: Original
	router.POST("/requestUploadUrl/:branch", handlers.RequestUploadUrlHandler)

	// Format 2: Direct requestUploadUrl
	router.POST("/requestUploadUrl-v2/:branch", func(c *gin.Context) {
		// Get the original parameters
		branchName := c.Param("branch")
		runtimeVersion := c.Query("runtimeVersion")

		// Get JSON request body
		var request handlers.FileNamesRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
			return
		}

		// Generate update ID
		updateId := uuid.New().String()

		// Get bucket
		resolvedBucket := bucket.GetBucket()
		requests, err := resolvedBucket.RequestUploadUrlsForFileUpdates(branchName, runtimeVersion, updateId, request.FileNames)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error requesting upload URLs"})
			return
		}

		// Create a different response format
		urls := make(map[string]string)
		for _, req := range requests {
			fileName := strings.TrimPrefix(req.Path, fmt.Sprintf("updates/%s/%s/%s/", branchName, runtimeVersion, updateId))
			urls[fileName] = req.Url
		}

		// Different format
		c.JSON(http.StatusOK, gin.H{
			"requestUploadUrl": urls[request.FileNames[0]],
			"updateId":         updateId,
		})
	})

	// Format 3: Try array format
	router.POST("/requestUploadUrl-v3/:branch", func(c *gin.Context) {
		// Get the original parameters
		branchName := c.Param("branch")
		runtimeVersion := c.Query("runtimeVersion")

		// Get JSON request body
		var request handlers.FileNamesRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
			return
		}

		// Generate update ID
		updateId := uuid.New().String()

		// Get bucket
		resolvedBucket := bucket.GetBucket()
		requests, err := resolvedBucket.RequestUploadUrlsForFileUpdates(branchName, runtimeVersion, updateId, request.FileNames)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error requesting upload URLs"})
			return
		}

		// Direct format - Just a string with the URL
		if len(requests) > 0 {
			c.String(http.StatusOK, requests[0].Url)
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "No upload URLs generated"})
		}
	})

	// Another format that might be compatible with eoas
	router.POST("/requestUploadUrls/:branch", func(c *gin.Context) {
		handlers.RequestUploadUrlHandler(c)
	})

	// API routes
	api := router.Group("/api")
	{
		// Dashboard routes
		api.GET("/dashboard/settings", handlers.GetSettingsHandler)
		api.GET("/dashboard/branches", handlers.GetBranchesHandler)
		api.GET("/dashboard/runtime-versions/:branch", handlers.GetRuntimeVersionsHandler)
		api.GET("/dashboard/updates/:branch/:runtimeVersion", handlers.GetUpdatesHandler)

		// Update routes
		api.POST("/update/upload/:branch", middleware.AuthMiddleware, handlers.UploadHandler)
		api.POST("/update/request-upload-url/:branch", middleware.AuthMiddleware, handlers.RequestUploadUrlHandler)
		api.POST("/update/request-upload-urls/:branch", handlers.RequestUploadUrlHandler)
		api.POST("/update/mark-uploaded/:branch", middleware.AuthMiddleware, handlers.MarkUpdateAsUploadedHandler)
		api.GET("/update/manifest/:branch/:runtimeVersion", handlers.ManifestHandler)
		api.GET("/update/assets/:path", handlers.AssetsHandler)
	}

	dashboardPath := getDashboardPath()

	if dashboard.IsDashboardEnabled() {
		router.GET("/dashboard/*path", func(c *gin.Context) {
			// Get env.js
			if c.Param("path") == "/env.js" {
				c.Header("Content-Type", "application/javascript")
				baseURL := config.GetEnv("BASE_URL")
				if baseURL == "" {
					baseURL = "http://localhost:3000"
				}
				c.String(200, fmt.Sprintf("window.env = { VITE_OTA_API_URL: '%s' };", baseURL))
				return
			}
			if c.Param("path") == "/" {
				target := "/dashboard/"
				if c.Request.URL.RawQuery != "" {
					target += "?" + c.Request.URL.RawQuery
				}
				c.Redirect(301, target)
				return
			}
			staticExtensions := []string{".css", ".js", ".svg", ".png", ".json", ".ico"}
			for _, ext := range staticExtensions {
				if len(c.Param("path")) > len(ext) && c.Param("path")[len(c.Param("path"))-len(ext):] == ext {
					filePath := filepath.Join(dashboardPath, c.Param("path")[len("/dashboard/"):])
					fmt.Println("Serving file", filePath)
					c.File(filePath)
					return
				}
			}
			filePath := filepath.Join(dashboardPath, "index.html")
			fmt.Println("Serving file", filePath)
			c.File(filePath)
		})
	}

	return router
}
