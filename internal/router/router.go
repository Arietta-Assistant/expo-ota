package infrastructure

import (
	"expo-open-ota/config"
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
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Debug logging middleware
func debugLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := uuid.New().String()

		// Log request details
		log.Printf("[RequestID: %s] Incoming %s request: %s",
			requestID, c.Request.Method, c.Request.URL.Path)

		// Log headers
		log.Printf("[RequestID: %s] Request headers:", requestID)
		for k, v := range c.Request.Header {
			log.Printf("[RequestID: %s]   %s: %v", requestID, k, v)
		}

		// Set request ID in context
		c.Set("requestID", requestID)

		// Track processing time
		startTime := time.Now()

		// Process request
		c.Next()

		// Log response details
		duration := time.Since(startTime)
		log.Printf("[RequestID: %s] Request completed in %v with status %d",
			requestID, duration, c.Writer.Status())
	}
}

// SetupRoutes configures all routes on the provided router
func SetupRoutes(router *gin.Engine) {
	// Health check endpoint must be at the top level
	router.GET("/health", handlers.HealthHandler)

	// Metrics endpoint
	router.GET("/metrics", func(c *gin.Context) {
		metrics.PrometheusHandler().ServeHTTP(c.Writer, c.Request)
	})

	// Authentication routes for dashboard
	router.POST("/auth/login", func(c *gin.Context) {
		log.Printf("Auth login request received")
		handlers.LoginHandler(c.Writer, c.Request)
	})

	router.POST("/auth/refresh", func(c *gin.Context) {
		log.Printf("Auth refresh request received")
		handlers.RefreshTokenHandler(c.Writer, c.Request)
	})

	// Add the /auth/refreshToken endpoint that matches what the client expects
	router.POST("/auth/refreshToken", func(c *gin.Context) {
		log.Printf("Auth refreshToken request received")
		handlers.RefreshTokenHandler(c.Writer, c.Request)
	})

	// API routes
	api := router.Group("/api")
	{
		// Dashboard API routes
		api.GET("/dashboard/settings", handlers.GetSettingsHandler)
		api.GET("/dashboard/branches", handlers.GetBranchesHandler)
		api.GET("/dashboard/runtime-versions/:branch", handlers.GetRuntimeVersionsHandler)
		api.GET("/dashboard/updates/:branch/:runtimeVersion", handlers.GetUpdatesHandler)

		// Aliases for dashboard endpoints (to match client expectations)
		api.GET("/settings", handlers.GetSettingsHandler)
		api.GET("/branches", handlers.GetBranchesHandler)
		api.GET("/runtime-versions/:branch", handlers.GetRuntimeVersionsHandler)
		api.GET("/updates/:branch/:runtimeVersion", handlers.GetUpdatesHandler)

		// Update API routes
		api.POST("/update/upload/:branch", middleware.AuthMiddleware, handlers.UploadHandler)
		api.POST("/update/request-upload-url/:branch", middleware.AuthMiddleware, handlers.RequestUploadUrlHandler)
		api.POST("/update/request-upload-urls/:branch", handlers.RequestUploadUrlHandler)
		api.POST("/update/mark-uploaded/:branch", middleware.AuthMiddleware, handlers.MarkUpdateAsUploadedHandler)
		api.GET("/update/manifest/:branch/:runtimeVersion", debugLoggerMiddleware(), handlers.ManifestHandler)
		api.GET("/update/assets/:path", debugLoggerMiddleware(), handlers.AssetsHandler)
		api.GET("/update/assets", debugLoggerMiddleware(), handlers.AssetsHandler)
		api.GET("/debug/updates/:branch/:runtimeVersion", handlers.ListUpdatesHandler)
	}

	// Special asset routes needed by Expo Updates
	// This adds compatibility with how Expo's fetchUpdateAsync looks for assets
	router.GET("/assets/:branch/:runtimeVersion/:updateId/*assetPath", debugLoggerMiddleware(), func(c *gin.Context) {
		branch := c.Param("branch")
		runtimeVersion := c.Param("runtimeVersion")
		updateId := c.Param("updateId")
		assetPath := c.Param("assetPath")

		// Strip leading slash from assetPath if present
		if strings.HasPrefix(assetPath, "/") {
			assetPath = assetPath[1:]
		}

		platform := c.DefaultQuery("platform", "ios") // Default to iOS if not specified

		requestID := c.GetString("requestID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		log.Printf("[RequestID: %s] Direct asset request: %s/%s/%s/%s (platform: %s)",
			requestID, branch, runtimeVersion, updateId, assetPath, platform)

		// This works by reusing the path parameter handler in AssetsHandler
		// We format the path to match what AssetsHandler expects
		completePath := fmt.Sprintf("%s/%s/%s/%s", branch, runtimeVersion, updateId, assetPath)
		c.Params = append(c.Params, gin.Param{Key: "path", Value: completePath})

		// Set the platform as a query parameter
		c.Request.URL.RawQuery = fmt.Sprintf("platform=%s", platform)

		handlers.AssetsHandler(c)
	})

	// Dashboard frontend route
	if dashboard.IsDashboardEnabled() {
		log.Println("Dashboard is enabled, registering dashboard routes")
		// Static file server for dashboard
		router.GET("/dashboard/*path", func(c *gin.Context) {
			path := c.Param("path")
			log.Printf("Dashboard request: %s", path)

			// Handle env.js special case
			if path == "/env.js" {
				baseURL := config.GetEnv("BASE_URL")
				if baseURL == "" {
					// Try to use the current request's host if BASE_URL is not set
					host := c.Request.Host
					proto := "http"
					if c.Request.TLS != nil || c.Request.Header.Get("X-Forwarded-Proto") == "https" {
						proto = "https"
					}
					baseURL = fmt.Sprintf("%s://%s", proto, host)
					log.Printf("BASE_URL not set, using request host instead: %s", baseURL)
				}
				c.Header("Content-Type", "application/javascript")
				c.String(http.StatusOK, fmt.Sprintf("window.env = { VITE_OTA_API_URL: '%s' };", baseURL))
				log.Printf("Served env.js with BASE_URL: %s", baseURL)
				return
			}

			// Get dashboard path
			dashPath := getDashboardPath()
			log.Printf("Using dashboard path: %s", dashPath)

			// Handle static files
			filePath := filepath.Join(dashPath, strings.TrimPrefix(path, "/"))
			log.Printf("Looking for file: %s", filePath)

			fileInfo, err := os.Stat(filePath)
			if err == nil && !fileInfo.IsDir() {
				log.Printf("Serving file: %s", filePath)
				c.File(filePath)
				return
			} else if err != nil {
				log.Printf("File not found: %s - %v", filePath, err)
			} else if fileInfo.IsDir() {
				log.Printf("Path is a directory: %s", filePath)
			}

			// Serve index.html for SPA
			indexPath := filepath.Join(dashPath, "index.html")
			log.Printf("Serving index.html: %s", indexPath)
			if _, err := os.Stat(indexPath); err != nil {
				log.Printf("Index.html not found: %v", err)
				c.String(http.StatusNotFound, "Dashboard not found")
				return
			}

			c.File(indexPath)
		})
	} else {
		log.Println("Dashboard is disabled, dashboard routes will not be registered")
	}
}

// NewRouter creates a new Gin router with all application routes
func NewRouter() *gin.Engine {
	router := gin.Default()
	SetupRoutes(router)
	return router
}

// getDashboardPath determines the appropriate path for dashboard files
func getDashboardPath() string {
	// Standard path for Railway
	railwayPath := "/app/dashboard/dist"
	if _, err := os.Stat(railwayPath); err == nil {
		log.Println("Using dashboard path:", railwayPath)
		return railwayPath
	} else {
		log.Printf("Railway dashboard path not found at %s: %v", railwayPath, err)
	}

	// For local development
	workingDir, err := os.Getwd()
	if err != nil {
		log.Printf("Failed to get working directory: %v", err)
		log.Println("Falling back to Railway path:", railwayPath)
		return railwayPath // Fallback to Railway path
	}

	localPath := filepath.Join(workingDir, "dashboard", "dist")
	if _, err := os.Stat(localPath); err == nil {
		log.Printf("Using local dashboard path: %s", localPath)
		// List files in the directory to verify content
		files, listErr := os.ReadDir(localPath)
		if listErr == nil {
			log.Printf("Found %d files in dashboard directory", len(files))
			for i, file := range files {
				if i < 5 { // Only log up to 5 files to avoid overwhelming logs
					log.Printf("Dashboard file: %s", file.Name())
				}
			}
		} else {
			log.Printf("Could not list files in dashboard directory: %v", listErr)
		}
	} else {
		log.Printf("Local dashboard path not found at %s: %v", localPath, err)
	}

	return localPath
}
