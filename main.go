package main

import (
	"expo-open-ota/config"
	infrastructure "expo-open-ota/internal/router"
	"expo-open-ota/internal/update"
	"log"
	"os"
	"time"
)

func main() {
	// Load configuration
	log.Println("Starting server initialization...")
	log.Println("Loading configuration...")
	config.LoadConfig()

	// Log important environment variables (without exposing secrets)
	logEnvironmentStatus()

	// Initialize router
	log.Println("Initializing router...")
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
		log.Println("PORT environment variable not set, using default:", port)
	}

	// Start server
	log.Printf("Server is running on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}

// logEnvironmentStatus logs the status of key environment variables
func logEnvironmentStatus() {
	// Check and log dashboard status
	dashboardEnabled := config.GetEnv("USE_DASHBOARD")
	log.Printf("Dashboard enabled: %s", dashboardEnabled)

	if dashboardEnabled == "true" {
		// Check dashboard files exist
		if _, err := os.Stat("/app/dashboard/dist"); err == nil {
			log.Println("Dashboard files found at /app/dashboard/dist")
		} else {
			log.Printf("WARNING: Dashboard files not found at /app/dashboard/dist: %v", err)
		}

		// Check admin password
		if config.GetEnv("ADMIN_PASSWORD") == "" {
			log.Println("WARNING: ADMIN_PASSWORD not set but dashboard is enabled")
		} else {
			log.Println("ADMIN_PASSWORD is set")
		}
	}

	// Log base URL
	baseURL := config.GetEnv("BASE_URL")
	log.Printf("BASE_URL: %s", baseURL)

	// Log storage configuration
	storageMode := config.GetEnv("STORAGE_MODE")
	log.Printf("Storage mode: %s", storageMode)

	// Log keys storage configuration
	keysStorageType := config.GetEnv("KEYS_STORAGE_TYPE")
	log.Printf("Keys storage type: %s", keysStorageType)
}
