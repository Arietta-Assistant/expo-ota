package middleware

import (
	"expo-open-ota/internal/auth"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func LoggingMiddleware(c *gin.Context) {
	log.Printf("Incoming request: %s %s", c.Request.Method, c.Request.URL.Path)
	c.Next()
}

func AuthMiddleware(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		decodedToken, err := auth.VerifyFirebaseToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Firebase token"})
			c.Abort()
			return
		}

		// Add user info to context if token is provided
		c.Set("user", decodedToken)
	}
	c.Next()
}
