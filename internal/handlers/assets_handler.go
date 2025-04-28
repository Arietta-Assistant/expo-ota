package handlers

import (
	"expo-open-ota/internal/assets"
	"log"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func AssetsHandler(c *gin.Context) {
	requestID := uuid.New().String()
	log.Printf("[RequestID: %s] ASSET-DEBUG: Starting asset request", requestID)

	// Log all headers for debugging
	log.Printf("[RequestID: %s] ASSET-DEBUG: Request headers:", requestID)
	for k, v := range c.Request.Header {
		log.Printf("[RequestID: %s]   %s: %v", requestID, k, v)
	}

	// Get required parameters
	assetName := c.Query("asset")
	runtimeVersion := c.Query("runtimeVersion")
	platform := c.Query("platform")
	branch := c.Query("branch")

	// If query parameters are empty, try path parameters
	if assetName == "" {
		assetName = c.Param("assetPath")
		runtimeVersion = c.Param("runtimeVersion")
		platform = c.Param("platform")
		branch = c.Param("branch")
	}

	// If still no branch, try to get it from the channel name header
	if branch == "" {
		branch = c.GetHeader("expo-channel-name")
		log.Printf("[RequestID: %s] ASSET-DEBUG: Using channel name as branch: %s", requestID, branch)
	}

	log.Printf("[RequestID: %s] ASSET-DEBUG: Request parameters - asset: %s, runtimeVersion: %s, platform: %s, branch: %s",
		requestID, assetName, runtimeVersion, platform, branch)

	// Validate parameters
	if assetName == "" || runtimeVersion == "" || platform == "" || branch == "" {
		log.Printf("[RequestID: %s] ASSET-DEBUG: Missing required parameters - asset: %s, runtimeVersion: %s, platform: %s, branch: %s",
			requestID, assetName, runtimeVersion, platform, branch)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required parameters"})
		return
	}

	// Create request
	req := assets.AssetsRequest{
		Branch:         branch,
		AssetName:      assetName,
		RuntimeVersion: runtimeVersion,
		Platform:       platform,
		RequestID:      requestID,
	}

	// Handle the asset request
	resp, err := assets.HandleAssetsWithFile(req)
	if err != nil {
		log.Printf("[RequestID: %s] ASSET-DEBUG: Error handling asset request: %v", requestID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error handling asset request"})
		return
	}

	// Set response headers
	for key, value := range resp.Headers {
		c.Header(key, value)
	}

	// Set content type based on file extension
	ext := filepath.Ext(assetName)
	switch ext {
	case ".js", ".hbc":
		c.Header("Content-Type", "application/javascript")
	case ".png":
		c.Header("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		c.Header("Content-Type", "image/jpeg")
	case ".gif":
		c.Header("Content-Type", "image/gif")
	case ".json":
		c.Header("Content-Type", "application/json")
	default:
		c.Header("Content-Type", "application/octet-stream")
	}

	// Add required Expo headers
	c.Header("expo-protocol-version", "1")
	c.Header("expo-sfv-version", "0")
	c.Header("Cache-Control", "private, max-age=0")

	// Log response details
	log.Printf("[RequestID: %s] ASSET-DEBUG: Sending response - Status: %d, Content-Type: %s, Headers: %v",
		requestID, resp.StatusCode, c.GetHeader("Content-Type"), c.Writer.Header())

	// Send response
	if resp.StatusCode != http.StatusOK {
		log.Printf("[RequestID: %s] ASSET-DEBUG: Non-200 status code: %d", requestID, resp.StatusCode)
		c.Status(resp.StatusCode)
		if len(resp.Body) > 0 {
			c.Writer.Write(resp.Body)
		}
		return
	}

	// For successful responses, write the body
	if len(resp.Body) > 0 {
		log.Printf("[RequestID: %s] ASSET-DEBUG: Writing response body (%d bytes)", requestID, len(resp.Body))
		c.Writer.Write(resp.Body)
	} else {
		log.Printf("[RequestID: %s] ASSET-DEBUG: Empty response body", requestID)
	}
}
