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
			// Log the error but don't block the request
			log.Printf("WARNING: Token verification failed: %v", err)

			// Set a dummy user in development environments
			if strings.Contains(c.Request.Host, "localhost") {
				log.Printf("Running in local development mode, allowing request to proceed")
				c.Set("user", "developer")
				c.Next()
				return
			}

			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Firebase token"})
			c.Abort()
			return
		}

		// Add user info to context if token is provided
		c.Set("user", decodedToken)
	} else {
		// In development environments, allow requests without auth headers
		if strings.Contains(c.Request.Host, "localhost") {
			log.Printf("No auth header but running in local development mode, allowing request")
			c.Set("user", "developer")
		}
	}
	c.Next()
}
