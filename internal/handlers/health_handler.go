package handlers

import (
	"github.com/gin-gonic/gin"
)

func HealthHandler(c *gin.Context) {
	c.String(200, "OK")
}
