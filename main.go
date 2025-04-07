package main

import (
	"expo-open-ota/config"
	infrastructure "expo-open-ota/internal/router"
	"log"
)

func main() {
	// Load configuration
	config.LoadConfig()

	// Initialize router
	router := infrastructure.NewRouter()

	// Get port from environment or use default
	port := config.GetEnv("PORT")
	if port == "" {
		port = "3000"
	}

	// Start server
	log.Printf("Server is running on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
