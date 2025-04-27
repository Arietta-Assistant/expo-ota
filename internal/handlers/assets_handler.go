package handlers

import (
	"expo-open-ota/internal/bucket"
	"expo-open-ota/internal/config"
	"expo-open-ota/internal/update"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func AssetsHandler(c *gin.Context) {
	// Get the path from the URL
	path := c.Param("path")

	if path == "" {
		// If path param is empty, try to get it from query parameters (for backward compatibility)
		assetPath := c.Query("asset")
		runtimeVersion := c.Query("runtimeVersion")
		platform := c.Query("platform")

		log.Printf("Asset request via query params: asset=%s, runtimeVersion=%s, platform=%s",
			assetPath, runtimeVersion, platform)

		if assetPath == "" || runtimeVersion == "" || platform == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required parameters: asset, runtimeVersion, or platform"})
			return
		}

		// Get the latest update for this runtime version
		update, err := update.GetLatestUpdateBundlePathForRuntimeVersion("ota-updates", runtimeVersion, "")
		if err != nil || update == nil {
			log.Printf("Error getting update for runtime version %s: %v", runtimeVersion, err)
			c.JSON(http.StatusNotFound, gin.H{"error": "Update not found"})
			return
		}

		// Construct the path using the branch, runtimeVersion, and updateId from the retrieved update
		path = strings.Join([]string{update.Branch, update.RuntimeVersion, update.UpdateId, assetPath}, "/")
		log.Printf("Constructed path from query parameters: %s", path)
	}

	// Parse the path to get branch, runtimeVersion, updateId, and fileName
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path format"})
		return
	}

	branch := parts[0]
	runtimeVersion := parts[1]
	updateId := parts[2]
	fileName := strings.Join(parts[3:], "/")

	// Log the request for debugging
	log.Printf("Serving asset: branch=%s, runtimeVersion=%s, updateId=%s, fileName=%s",
		branch, runtimeVersion, updateId, fileName)

	// Get bucket type from environment
	bucketType := config.GetEnv("BUCKET_TYPE")
	if bucketType == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "BUCKET_TYPE not configured"})
		return
	}

	// Get the appropriate bucket implementation
	var b bucket.Bucket
	var err error
	switch bucketType {
	case "local":
		b = &bucket.LocalBucket{BasePath: config.GetEnv("LOCAL_BUCKET_BASE_PATH")}
	case "s3":
		b = &bucket.S3Bucket{BucketName: config.GetEnv("S3_BUCKET_NAME")}
	case "firebase":
		b, err = bucket.NewFirebaseBucket()
		if err != nil {
			log.Printf("Error creating Firebase bucket: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize storage"})
			return
		}
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unsupported bucket type"})
		return
	}

	// Get the file from the bucket
	file, err := b.GetFile(branch, runtimeVersion, updateId, fileName)
	if err != nil {
		log.Printf("Error getting file %s/%s/%s/%s: %v", branch, runtimeVersion, updateId, fileName, err)

		// Try alternative paths for known problematic files
		if strings.Contains(fileName, "_expo/") {
			// Try without the _expo prefix
			alternativePath := strings.TrimPrefix(fileName, "_expo/")
			log.Printf("Trying alternative path without _expo/ prefix: %s", alternativePath)
			file, err = b.GetFile(branch, runtimeVersion, updateId, alternativePath)
			if err != nil {
				log.Printf("Alternative path also failed: %v", err)
			} else {
				log.Printf("Successfully found file using alternative path: %s", alternativePath)
				goto serve_file // Skip to serving the file
			}
		}

		if strings.HasPrefix(fileName, "assets/") {
			// Try without the assets/ prefix
			alternativePath := strings.TrimPrefix(fileName, "assets/")
			log.Printf("Trying alternative path without assets/ prefix: %s", alternativePath)
			file, err = b.GetFile(branch, runtimeVersion, updateId, alternativePath)
			if err != nil {
				log.Printf("Alternative path also failed: %v", err)
			} else {
				log.Printf("Successfully found file using alternative path: %s", alternativePath)
				goto serve_file // Skip to serving the file
			}
		}

		// If all attempts failed
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

serve_file:
	defer file.Close()

	// Set appropriate headers based on file type
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".js":
		c.Header("Content-Type", "application/javascript")
	case ".json":
		c.Header("Content-Type", "application/json")
	case ".png":
		c.Header("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		c.Header("Content-Type", "image/jpeg")
	case ".hbc":
		c.Header("Content-Type", "application/octet-stream")
	default:
		c.Header("Content-Type", "application/octet-stream")
	}

	// Stream the file to the response
	c.Stream(func(w io.Writer) bool {
		_, err := io.Copy(w, file)
		return err == nil
	})
}
