package infrastructure

import (
	"expo-open-ota/config"
	"expo-open-ota/internal/bucket"
	"expo-open-ota/internal/dashboard"
	"expo-open-ota/internal/handlers"
	"expo-open-ota/internal/metrics"
	"expo-open-ota/internal/middleware"
	"expo-open-ota/internal/update"
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
	// Simple implementation that should work in most environments
	workingDir, err := os.Getwd()
	if err != nil {
		log.Printf("Error getting working directory: %v", err)
		return "/app/dashboard/dist" // Railway default path
	}

	// For Railway and production environments
	if _, err := os.Stat("/app/dashboard/dist"); err == nil {
		return "/app/dashboard/dist"
	}

	// For local development
	return filepath.Join(workingDir, "dashboard", "dist")
}

func NewRouter() *gin.Engine {
	router := gin.Default()
	router.Use(middleware.LoggingMiddleware)

	// Add request logging middleware
	router.Use(func(c *gin.Context) {
		log.Printf("DEBUG: Incoming request: %s %s, Params: %v, Headers: %v",
			c.Request.Method,
			c.Request.URL.Path,
			c.Params,
			c.Request.Header)
		c.Next()
	})

	// Health check
	router.GET("/health", handlers.HealthHandler)

	// Debug endpoint to dump metadata for a specific update
	router.GET("/debug/metadata/:updateId", func(c *gin.Context) {
		updateId := c.Param("updateId")
		log.Printf("Manual request to dump metadata for update: %s", updateId)

		// Try direct access method first (more reliable)
		resolvedBucket := bucket.GetBucket()
		if firebaseBucket, ok := resolvedBucket.(*bucket.FirebaseBucket); ok {
			log.Printf("Using direct method to access metadata")
			// Call the method to log the metadata
			firebaseBucket.GetDirectMetadata(updateId)

			// Return a simple success message
			c.JSON(http.StatusOK, gin.H{
				"message":  "Metadata dump has been written to the logs",
				"updateId": updateId,
			})
			return
		}

		// Try to find the update in all branches and runtime versions
		branches, err := resolvedBucket.GetBranches()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error getting branches: %v", err)})
			return
		}

		for _, branch := range branches {
			runtimeVersions, err := resolvedBucket.GetRuntimeVersions(branch)
			if err != nil {
				continue
			}

			for _, rv := range runtimeVersions {
				updates, err := resolvedBucket.GetUpdates(branch, rv.RuntimeVersion)
				if err != nil {
					continue
				}

				for _, u := range updates {
					if u.UpdateId == updateId {
						// Found the update - dump its metadata
						metadata, err := update.GetMetadata(u)
						if err != nil {
							c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error getting metadata: %v", err)})
							return
						}

						// Return the metadata as JSON
						c.JSON(http.StatusOK, gin.H{
							"updateId":       updateId,
							"branch":         branch,
							"runtimeVersion": rv.RuntimeVersion,
							"metadata":       metadata,
							"extra":          metadata.MetadataJSON.Extra,
						})
						return
					}
				}
			}
		}

		// If we reach here, we didn't find the update
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Update with ID %s not found", updateId)})
	})

	// Debug endpoint to manually trigger the dump function
	router.GET("/debug/dump-metadata", func(c *gin.Context) {
		log.Println("Manually triggering metadata dump...")
		go update.DumpSpecificUpdateMetadata()
		c.JSON(http.StatusOK, gin.H{"message": "Dump process started, check logs for results"})
	})

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
		// Simple debug route
		router.GET("/dashboard-debug", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"path": getDashboardPath(),
			})
		})

		// Redirect /dashboard/dist/ to /dashboard/
		router.GET("/dashboard/dist/*any", func(c *gin.Context) {
			c.Redirect(http.StatusMovedPermanently, "/dashboard/")
		})

		router.GET("/dashboard/*path", func(c *gin.Context) {
			pathParam := c.Param("path")

			// Handle env.js
			if pathParam == "/env.js" {
				c.Header("Content-Type", "application/javascript")
				baseURL := config.GetEnv("BASE_URL")
				if baseURL == "" {
					baseURL = "http://localhost:3000"
				}
				c.String(200, fmt.Sprintf("window.env = { VITE_OTA_API_URL: '%s' };", baseURL))
				return
			}

			// Try to serve the file directly
			filePath := filepath.Join(dashboardPath, strings.TrimPrefix(pathParam, "/"))
			if fileExists(filePath) {
				c.File(filePath)
				return
			}

			// If not found or root path, serve index.html
			c.File(filepath.Join(dashboardPath, "index.html"))
		})
	}

	return router
}

// Helper functions for dashboard
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func getWorkingDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return dir
}

func getExecutablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return exe
}
