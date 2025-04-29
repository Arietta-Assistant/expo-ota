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

	log.Printf("[RequestID: %s] Query parameters - asset: %s, runtimeVersion: %s, platform: %s",
		requestID, assetPath, runtimeVersion, platform)

	// Extract FIREBASE_TOKEN from headers or Expo-Extra-Params
	firebaseToken := c.GetHeader("FIREBASE_TOKEN")

	// If not found in direct headers, try to extract from Expo-Extra-Params
	if firebaseToken == "" {
		extraParams := c.GetHeader("Expo-Extra-Params")
		if extraParams != "" {
			log.Printf("[RequestID: %s] Parsing Expo-Extra-Params for FIREBASE_TOKEN", requestID)
			extraParamsParts := strings.Split(extraParams, ",")
			for _, part := range extraParamsParts {
				part = strings.TrimSpace(part)
				if strings.Contains(part, "FIREBASE_TOKEN") {
					// Extract the value between quotes
					start := strings.Index(part, "\"")
					end := strings.LastIndex(part, "\"")
					if start != -1 && end != -1 && end > start {
						firebaseToken = part[start+1 : end]
						log.Printf("[RequestID: %s] Found FIREBASE_TOKEN in Expo-Extra-Params", requestID)
					}
					break
				}
			}
		}
	}

	if firebaseToken != "" {
		log.Printf("[RequestID: %s] FIREBASE_TOKEN is present for asset request (length: %d)",
			requestID, len(firebaseToken))
	}

	// Check if we're using path parameters instead
	path := c.Param("path")
	log.Printf("[RequestID: %s] Path parameter: %s", requestID, path)

	if path != "" {
		parts := strings.Split(path, "/")
		if len(parts) < 4 {
			log.Printf("[RequestID: %s] Invalid path format: %s", requestID, path)
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

		// Set required Expo headers
		c.Header("expo-protocol-version", "1")
		c.Header("expo-sfv-version", "0")
		c.Header("Cache-Control", "private, max-age=0")

		// Return appropriate response
		for key, value := range res.Headers {
			c.Header(key, value)
		}

		// Set content type based on asset type
		if strings.HasSuffix(assetPath, ".hbc") || strings.HasSuffix(assetPath, ".js") {
			c.Header("Content-Type", "application/javascript")
		} else if strings.HasSuffix(assetPath, ".png") {
			c.Header("Content-Type", "image/png")
		} else if strings.HasSuffix(assetPath, ".jpg") || strings.HasSuffix(assetPath, ".jpeg") {
			c.Header("Content-Type", "image/jpeg")
		} else if strings.HasSuffix(assetPath, ".gif") {
			c.Header("Content-Type", "image/gif")
		} else if strings.HasSuffix(assetPath, ".json") {
			c.Header("Content-Type", "application/json")
		} else {
			c.Header("Content-Type", "application/octet-stream")
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
		log.Printf("[RequestID: %s] Missing required parameters: asset=%s, runtimeVersion=%s, platform=%s",
			requestID, assetPath, runtimeVersion, platform)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required parameters: asset, runtimeVersion, or platform"})
		return
	}

	// Use default branch if not specified
	branch := c.Query("branch")
	if branch == "" {
		branch = "ota-updates" // Default branch
		log.Printf("[RequestID: %s] Using default branch: %s", requestID, branch)
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
	log.Printf("[RequestID: %s] Looking for asset: %s (platform: %s, runtimeVersion: %s, branch: %s)",
		requestID, assetPath, platform, runtimeVersion, branch)

	res, err := assets.HandleAssetsWithFile(req)
	if err != nil {
		log.Printf("[RequestID: %s] Error handling asset request: %v", requestID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Set required Expo headers
	c.Header("expo-protocol-version", "1")
	c.Header("expo-sfv-version", "0")
	c.Header("Cache-Control", "private, max-age=0")

	// Return appropriate response
	for key, value := range res.Headers {
		c.Header(key, value)
	}

	// Set content type based on asset type
	if strings.HasSuffix(assetPath, ".hbc") || strings.HasSuffix(assetPath, ".js") {
		c.Header("Content-Type", "application/javascript")
	} else if strings.HasSuffix(assetPath, ".png") {
		c.Header("Content-Type", "image/png")
	} else if strings.HasSuffix(assetPath, ".jpg") || strings.HasSuffix(assetPath, ".jpeg") {
		c.Header("Content-Type", "image/jpeg")
	} else if strings.HasSuffix(assetPath, ".gif") {
		c.Header("Content-Type", "image/gif")
	} else if strings.HasSuffix(assetPath, ".json") {
		c.Header("Content-Type", "application/json")
	} else {
		c.Header("Content-Type", "application/octet-stream")
	}

	if res.StatusCode != http.StatusOK {
		log.Printf("[RequestID: %s] Non-200 status code: %d, body: %s", requestID, res.StatusCode, string(res.Body))
		c.JSON(res.StatusCode, gin.H{"error": string(res.Body)})
		return
	}

	c.Data(res.StatusCode, res.ContentType, res.Body)
}
