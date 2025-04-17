package infrastructure

import (
	"expo-open-ota/config"
	"expo-open-ota/internal/dashboard"
	"expo-open-ota/internal/handlers"
	"expo-open-ota/internal/metrics"
	"expo-open-ota/internal/middleware"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// SetupRoutes configures all routes on the provided router
func SetupRoutes(router *gin.Engine) {
	// Health check endpoint must be at the top level
	router.GET("/health", handlers.HealthHandler)

	// Metrics endpoint
	router.GET("/metrics", func(c *gin.Context) {
		metrics.PrometheusHandler().ServeHTTP(c.Writer, c.Request)
	})

	// API routes
	api := router.Group("/api")
	{
		// Dashboard API routes
		api.GET("/dashboard/settings", handlers.GetSettingsHandler)
		api.GET("/dashboard/branches", handlers.GetBranchesHandler)
		api.GET("/dashboard/runtime-versions/:branch", handlers.GetRuntimeVersionsHandler)
		api.GET("/dashboard/updates/:branch/:runtimeVersion", handlers.GetUpdatesHandler)

		// Update API routes
		api.POST("/update/upload/:branch", middleware.AuthMiddleware, handlers.UploadHandler)
		api.POST("/update/request-upload-url/:branch", middleware.AuthMiddleware, handlers.RequestUploadUrlHandler)
		api.POST("/update/request-upload-urls/:branch", handlers.RequestUploadUrlHandler)
		api.POST("/update/mark-uploaded/:branch", middleware.AuthMiddleware, handlers.MarkUpdateAsUploadedHandler)
		api.GET("/update/manifest/:branch/:runtimeVersion", handlers.ManifestHandler)
		api.GET("/update/assets/:path", handlers.AssetsHandler)
	}

	// Dashboard frontend route
	if dashboard.IsDashboardEnabled() {
		// Static file server for dashboard
		router.GET("/dashboard/*path", func(c *gin.Context) {
			path := c.Param("path")

			// Handle env.js special case
			if path == "/env.js" {
				baseURL := config.GetEnv("BASE_URL")
				if baseURL == "" {
					baseURL = "http://localhost:3000"
				}
				c.Header("Content-Type", "application/javascript")
				c.String(http.StatusOK, fmt.Sprintf("window.env = { VITE_OTA_API_URL: '%s' };", baseURL))
				return
			}

			// Get dashboard path
			dashPath := getDashboardPath()

			// Handle static files
			filePath := filepath.Join(dashPath, strings.TrimPrefix(path, "/"))
			fileInfo, err := os.Stat(filePath)
			if err == nil && !fileInfo.IsDir() {
				c.File(filePath)
				return
			}

			// Serve index.html for SPA
			c.File(filepath.Join(dashPath, "index.html"))
		})
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
	if _, err := os.Stat("/app/dashboard/dist"); err == nil {
		return "/app/dashboard/dist"
	}

	// For local development
	workingDir, err := os.Getwd()
	if err != nil {
		return "/app/dashboard/dist" // Fallback to Railway path
	}

	return filepath.Join(workingDir, "dashboard", "dist")
}
