package handlers

import (
	"expo-open-ota/internal/update"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

func MarkUpdateAsUploadedHandler(c *gin.Context) {
	branchName := c.Param("branch")
	platform := c.Query("platform")
	runtimeVersion := c.Query("runtimeVersion")
	updateId := c.Query("updateId")

	if platform == "" || (platform != "ios" && platform != "android") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid platform"})
		return
	}

	if branchName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No branch provided"})
		return
	}

	if runtimeVersion == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No runtime version provided"})
		return
	}

	if updateId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No update id provided"})
		return
	}

	currentUpdate, err := update.GetUpdate(branchName, runtimeVersion, updateId)
	if err != nil {
		log.Printf("Error getting update: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error getting update"})
		return
	}

	err = update.MarkUpdateAsChecked(*currentUpdate)
	if err != nil {
		log.Printf("Error marking update as checked: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error marking update as checked"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
