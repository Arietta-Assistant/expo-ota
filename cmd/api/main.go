package main

import (
	"expo-open-ota/config"
	"expo-open-ota/internal/metrics"
	infrastructure "expo-open-ota/internal/router"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/handlers"
)

func init() {
	config.LoadConfig()
	metrics.InitMetrics()
	gin.SetMode(gin.ReleaseMode)
}

func main() {
	router := infrastructure.NewRouter()
	log.Println("Server is running on port 3000")
	corsOptions := handlers.CORS(
		handlers.AllowedHeaders([]string{"Authorization", "Content-Type"}),
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedOrigins([]string{"*"}),
		handlers.AllowCredentials(),
	)
	err := http.ListenAndServe(":3000", corsOptions(router))
	if err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
