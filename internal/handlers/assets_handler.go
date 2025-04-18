package handlers

import (
	"expo-open-ota/internal/bucket"
	"expo-open-ota/internal/config"
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "No path provided"})
		return
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
		log.Printf("Error getting file: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}
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
	default:
		c.Header("Content-Type", "application/octet-stream")
	}

	// Stream the file to the response
	c.Stream(func(w io.Writer) bool {
		_, err := io.Copy(w, file)
		return err == nil
	})
}
