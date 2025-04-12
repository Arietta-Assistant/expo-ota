package infrastructure

import (
	"expo-open-ota/config"
	"expo-open-ota/internal/dashboard"
	"expo-open-ota/internal/handlers"
	"expo-open-ota/internal/metrics"
	"expo-open-ota/internal/middleware"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
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
		api.POST("/update/request-upload-urls/:branch", middleware.AuthMiddleware, handlers.RequestUploadUrlHandler)
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
