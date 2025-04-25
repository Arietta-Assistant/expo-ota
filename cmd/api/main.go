package main

import (
	"expo-open-ota/config"
	"expo-open-ota/internal/metrics"
	infrastructure "expo-open-ota/internal/router"
	"log"
	"os"

	"github.com/gin-gonic/gin"
)

func init() {
	// Enable the dashboard
	os.Setenv("USE_DASHBOARD", "true")

	config.LoadConfig()
	metrics.InitMetrics()
	gin.SetMode(gin.ReleaseMode)
}

func main() {
	// Create a new Gin engine
	router := gin.Default()

	// Add CORS middleware
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Setup routes using the router package
	infrastructure.SetupRoutes(router)

	log.Println("Server is running on port 8080")
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
