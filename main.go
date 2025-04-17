package main

import (
	"expo-open-ota/config"
	infrastructure "expo-open-ota/internal/router"
	"expo-open-ota/internal/update"
	"log"
	"time"
)

func main() {
	// Load configuration
	config.LoadConfig()

	// Initialize router
	router := infrastructure.NewRouter()

	// Dump metadata for the specific update
	go func() {
		// Wait a bit to make sure everything is initialized
		time.Sleep(2 * time.Second)
		log.Println("Starting metadata dump for specific update...")
		update.DumpSpecificUpdateMetadata()
		log.Println("Metadata dump complete")
	}()

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
