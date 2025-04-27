package handlers

import (
	"expo-open-ota/internal/bucket"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

func ListUpdatesHandler(c *gin.Context) {
	branch := c.Param("branch")
	runtimeVersion := c.Param("runtimeVersion")

	if branch == "" || runtimeVersion == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Branch and runtimeVersion parameters are required"})
		return
	}

	log.Printf("DEBUG: Listing updates for branch=%s, runtimeVersion=%s", branch, runtimeVersion)

	// Get bucket instance
	b := bucket.GetBucket()

	// Call ListUpdates method
	updates, err := b.ListUpdates(branch, runtimeVersion)
	if err != nil {
		log.Printf("ERROR: Failed to list updates: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list updates", "details": err.Error()})
		return
	}

	// Return the list of updates
	c.JSON(http.StatusOK, gin.H{
		"branch":         branch,
		"runtimeVersion": runtimeVersion,
		"updates":        updates,
	})
}
