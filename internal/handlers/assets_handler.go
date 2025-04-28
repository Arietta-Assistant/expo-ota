package handlers

import (
	"expo-open-ota/internal/assets"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func AssetsHandler(c *gin.Context) {
	// Create a unique request ID for tracing
	requestID := uuid.New().String()
	log.Printf("[RequestID: %s] Processing asset request", requestID)

	// Get the asset path from query parameters
	assetPath := c.Query("asset")
	runtimeVersion := c.Query("runtimeVersion")
	platform := c.Query("platform")

	// Check if we're using path parameters instead
	path := c.Param("path")
	if path != "" {
		parts := strings.Split(path, "/")
		if len(parts) < 4 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path format"})
			return
		}

		// Parse the path components
		branch := parts[0]
		runtimeVersion = parts[1]
		updateId := parts[2]
		assetPath = strings.Join(parts[3:], "/")

		// Log the path components
		log.Printf("[RequestID: %s] Path request: branch=%s, runtimeVersion=%s, updateId=%s, assetPath=%s",
			requestID, branch, runtimeVersion, updateId, assetPath)

		// Create a specific request with the update ID already known
		req := assets.AssetsRequest{
			Branch:         branch,
			AssetName:      assetPath,
			RuntimeVersion: runtimeVersion,
			Platform:       platform,
			RequestID:      requestID,
		}

		// Handle the request
		res, err := assets.HandleAssetsWithFile(req)
		if err != nil {
			log.Printf("[RequestID: %s] Error handling asset request: %v", requestID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}

		// Return appropriate response
		for key, value := range res.Headers {
			c.Header(key, value)
		}

		if res.StatusCode != http.StatusOK {
			c.Data(res.StatusCode, res.ContentType, res.Body)
			return
		}

		c.Data(res.StatusCode, res.ContentType, res.Body)
		return
	}

	// For query parameter requests (the common case)
	if assetPath == "" || runtimeVersion == "" || platform == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required parameters: asset, runtimeVersion, or platform"})
		return
	}

	// Use default branch if not specified
	branch := c.Query("branch")
	if branch == "" {
		branch = "ota-updates" // Default branch
	}

	// Create the request object
	req := assets.AssetsRequest{
		Branch:         branch,
		AssetName:      assetPath,
		RuntimeVersion: runtimeVersion,
		Platform:       platform,
		RequestID:      requestID,
	}

	// Use our improved asset handling logic
	log.Printf("[RequestID: %s] Looking for asset: %s (platform: %s, runtimeVersion: %s)",
		requestID, assetPath, platform, runtimeVersion)

	res, err := assets.HandleAssetsWithFile(req)
	if err != nil {
		log.Printf("[RequestID: %s] Error handling asset request: %v", requestID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Return appropriate response
	for key, value := range res.Headers {
		c.Header(key, value)
	}

	if res.StatusCode != http.StatusOK {
		c.JSON(res.StatusCode, gin.H{"error": string(res.Body)})
		return
	}

	c.Data(res.StatusCode, res.ContentType, res.Body)
}
