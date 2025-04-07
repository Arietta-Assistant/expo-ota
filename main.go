package main

import (
	"expo-open-ota/config"
	"expo-open-ota/internal/handlers"
	"log"
	"net/http"
)

func main() {
	// Register health check endpoint
	http.HandleFunc("/health", handlers.HealthHandler)

	// ... register other endpoints ...

	port := config.GetEnv("PORT")
	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
