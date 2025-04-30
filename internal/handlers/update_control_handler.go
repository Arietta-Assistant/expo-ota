package handlers

import (
	"expo-open-ota/internal/bucket"
	"expo-open-ota/internal/update"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ActivateUpdateHandler handles the activation of an update
func ActivateUpdateHandler(c *gin.Context) {
	branch := c.Param("branch")
	runtimeVersion := c.Param("runtimeVersion")
	updateId := c.Param("updateId")

	log.Printf("Activating update: branch=%s, runtimeVersion=%s, updateId=%s",
		branch, runtimeVersion, updateId)

	err := update.ActivateUpdate(branch, runtimeVersion, updateId)
	if err != nil {
		log.Printf("Error activating update: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Update activated successfully",
	})
}

// DeactivateUpdateHandler handles the deactivation of an update
func DeactivateUpdateHandler(c *gin.Context) {
	branch := c.Param("branch")
	runtimeVersion := c.Param("runtimeVersion")
	updateId := c.Param("updateId")

	log.Printf("Deactivating update: branch=%s, runtimeVersion=%s, updateId=%s",
		branch, runtimeVersion, updateId)

	err := update.DeactivateUpdate(branch, runtimeVersion, updateId)
	if err != nil {
		log.Printf("Error deactivating update: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Update deactivated successfully",
	})
}

// DeleteUpdateHandler handles the deletion of an update
func DeleteUpdateHandler(c *gin.Context) {
	branch := c.Param("branch")
	runtimeVersion := c.Param("runtimeVersion")
	updateId := c.Param("updateId")

	log.Printf("Deleting update: branch=%s, runtimeVersion=%s, updateId=%s",
		branch, runtimeVersion, updateId)

	resolvedBucket := bucket.GetBucket()
	err := resolvedBucket.DeleteUpdateFolder(branch, runtimeVersion, updateId)
	if err != nil {
		log.Printf("Error deleting update: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Update deleted successfully",
	})
}

// GetUpdateStatsHandler gets download statistics for an update
func GetUpdateStatsHandler(c *gin.Context) {
	branch := c.Param("branch")
	runtimeVersion := c.Param("runtimeVersion")
	updateId := c.Param("updateId")

	log.Printf("Getting stats for update: branch=%s, runtimeVersion=%s, updateId=%s",
		branch, runtimeVersion, updateId)

	// Get download records from bucket storage
	resolvedBucket := bucket.GetBucket()
	downloads, err := resolvedBucket.GetUpdateDownloads(branch, runtimeVersion, updateId)
	if err != nil {
		log.Printf("Error getting update downloads: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Count unique users and devices
	users := make(map[string]bool)
	devices := make(map[string]bool)

	// Process downloads to get unique users and last download dates
	userStats := make([]map[string]string, 0)

	for _, download := range downloads {
		users[download.UserId] = true
		devices[download.DeviceId] = true

		// Add user info to the response
		userStat := map[string]string{
			"userId":           download.UserId,
			"deviceId":         download.DeviceId,
			"lastDownloadedAt": download.DownloadedAt,
		}
		userStats = append(userStats, userStat)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"downloadCount": len(downloads),
		"uniqueUsers":   len(users),
		"uniqueDevices": len(devices),
		"users":         userStats,
	})
}
