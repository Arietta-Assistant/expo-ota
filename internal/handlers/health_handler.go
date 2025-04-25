package handlers

import (
	"log"

	"github.com/gin-gonic/gin"
)

func HealthHandler(c *gin.Context) {
	log.Printf("Health check request received from %s", c.ClientIP())
	c.String(200, "OK")
	log.Printf("Health check response sent: 200 OK")
}
