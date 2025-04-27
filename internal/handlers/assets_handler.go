package handlers

import (
	"encoding/json"
	"expo-open-ota/internal/bucket"
	"expo-open-ota/internal/config"
	"expo-open-ota/internal/update"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func AssetsHandler(c *gin.Context) {
	// Get the path from the URL
	path := c.Param("path")

	var branch, runtimeVersion, updateId, fileName string

	if path == "" {
		// If path param is empty, try to get it from query parameters (for backward compatibility)
		assetPath := c.Query("asset")
		runtimeVersion = c.Query("runtimeVersion")
		platform := c.Query("platform")

		log.Printf("ASSET-REQUEST: Query params: asset=%s, runtimeVersion=%s, platform=%s",
			assetPath, runtimeVersion, platform)

		if assetPath == "" || runtimeVersion == "" || platform == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required parameters: asset, runtimeVersion, or platform"})
			return
		}

		branch = "ota-updates" // Default branch if not specified

		// Get the latest update for this runtime version
		update, err := update.GetLatestUpdateBundlePathForRuntimeVersion(branch, runtimeVersion, "")
		if err != nil || update == nil {
			log.Printf("ASSET-ERROR: No update for runtime %s: %v", runtimeVersion, err)
			c.JSON(http.StatusNotFound, gin.H{"error": "Update not found"})
			return
		}

		log.Printf("ASSET-INFO: Resolved update for asset request: Branch=%s, RuntimeVersion=%s, UpdateID=%s",
			update.Branch, update.RuntimeVersion, update.UpdateId)

		// Set variables for further processing
		branch = update.Branch
		updateId = update.UpdateId
		fileName = assetPath

		// Try to find the correct bundle path based on platform
		log.Printf("ASSET-INFO: Looking for bundle file based on platform: %s", platform)

		// Get bucket to access the metadata file
		bucket := bucket.GetBucket()
		metadataFile, metaErr := bucket.GetFile(branch, runtimeVersion, updateId, "metadata.json")
		if metaErr == nil {
			defer metadataFile.Close()

			// Read and parse the metadata JSON
			var metadataObj struct {
				FileMetadata struct {
					Android struct {
						Bundle string `json:"bundle"`
					} `json:"android"`
					IOS struct {
						Bundle string `json:"bundle"`
					} `json:"ios"`
				} `json:"fileMetadata"`
			}

			// Read metadata content
			metadataContent, readErr := io.ReadAll(metadataFile)
			if readErr == nil {
				log.Printf("ASSET-INFO: Successfully read metadata file for update %s", updateId)

				// Parse the JSON
				if jsonErr := json.Unmarshal(metadataContent, &metadataObj); jsonErr == nil {
					// Get the bundle path based on platform
					var bundlePath string
					if platform == "android" && metadataObj.FileMetadata.Android.Bundle != "" {
						bundlePath = metadataObj.FileMetadata.Android.Bundle
						log.Printf("ASSET-INFO: Android bundle path from metadata: %s", bundlePath)
					} else if platform == "ios" && metadataObj.FileMetadata.IOS.Bundle != "" {
						bundlePath = metadataObj.FileMetadata.IOS.Bundle
						log.Printf("ASSET-INFO: iOS bundle path from metadata: %s", bundlePath)
					}

					// If requested asset matches bundle path pattern but isn't exact, use the exact path
					if bundlePath != "" && strings.Contains(assetPath, "index") && strings.HasSuffix(assetPath, ".hbc") {
						log.Printf("ASSET-INFO: Replacing requested bundle path %s with exact path from metadata: %s", assetPath, bundlePath)
						fileName = bundlePath
					}
				} else {
					log.Printf("ASSET-WARN: Failed to parse metadata JSON: %v", jsonErr)
				}
			} else {
				log.Printf("ASSET-WARN: Failed to read metadata content: %v", readErr)
			}
		} else {
			log.Printf("ASSET-WARN: Could not read metadata file: %v", metaErr)
		}
	} else {
		// Parse the path to get branch, runtimeVersion, updateId, and fileName
		parts := strings.Split(path, "/")
		if len(parts) < 4 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path format"})
			return
		}

		branch = parts[0]
		runtimeVersion = parts[1]
		updateId = parts[2]
		fileName = strings.Join(parts[3:], "/")
	}

	// Log the request for debugging
	log.Printf("ASSET-REQUEST: branch=%s, runtimeVersion=%s, updateId=%s, fileName=%s",
		branch, runtimeVersion, updateId, fileName)

	// Get bucket type from environment
	bucketType := config.GetEnv("BUCKET_TYPE")
	if bucketType == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "BUCKET_TYPE not configured"})
		return
	}

	log.Printf("Using bucket type: %s", bucketType)

	// Get the appropriate bucket implementation
	var b bucket.Bucket
	var err error
	switch bucketType {
	case "local":
		basePath := config.GetEnv("LOCAL_BUCKET_BASE_PATH")
		log.Printf("Using local bucket with base path: %s", basePath)
		b = &bucket.LocalBucket{BasePath: basePath}
	case "s3":
		bucketName := config.GetEnv("S3_BUCKET_NAME")
		log.Printf("Using S3 bucket with name: %s", bucketName)
		b = &bucket.S3Bucket{BucketName: bucketName}
	case "firebase":
		log.Printf("Using Firebase bucket")
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

	// Track all attempted paths for debugging
	attemptedPaths := []string{}

	// Get the file from the bucket
	fullPath := strings.Join([]string{branch, runtimeVersion, updateId, fileName}, "/")
	attemptedPaths = append(attemptedPaths, fullPath)
	log.Printf("ASSET-INFO: Attempting to get file with original path: %s", fullPath)
	file, err := b.GetFile(branch, runtimeVersion, updateId, fileName)
	if err != nil {
		log.Printf("ASSET-ERROR: Error getting file %s/%s/%s/%s: %v", branch, runtimeVersion, updateId, fileName, err)

		// Show more information about the bucket for debugging
		resolvedBucket := bucket.GetBucket()
		log.Printf("ASSET-DEBUG: Using bucket type: %T", resolvedBucket)

		// Try alternative paths for known problematic files
		if strings.Contains(fileName, "_expo/") {
			// Try without the _expo prefix
			alternativePath := strings.TrimPrefix(fileName, "_expo/")
			log.Printf("ASSET-INFO: Trying alternative path without _expo/ prefix: %s", alternativePath)
			attemptedPaths = append(attemptedPaths, strings.Join([]string{branch, runtimeVersion, updateId, alternativePath}, "/"))
			file, err = b.GetFile(branch, runtimeVersion, updateId, alternativePath)
			if err != nil {
				log.Printf("ASSET-ERROR: Alternative path also failed: %v", err)
			} else {
				log.Printf("ASSET-SUCCESS: Found file using alternative path: %s", alternativePath)
				goto serve_file // Skip to serving the file
			}
		}

		if strings.HasPrefix(fileName, "assets/") {
			// Try without the assets/ prefix
			alternativePath := strings.TrimPrefix(fileName, "assets/")
			log.Printf("ASSET-INFO: Trying alternative path without assets/ prefix: %s", alternativePath)
			attemptedPaths = append(attemptedPaths, strings.Join([]string{branch, runtimeVersion, updateId, alternativePath}, "/"))
			file, err = b.GetFile(branch, runtimeVersion, updateId, alternativePath)
			if err != nil {
				log.Printf("ASSET-ERROR: Alternative path also failed: %v", err)
			} else {
				log.Printf("ASSET-SUCCESS: Found file using alternative path: %s", alternativePath)
				goto serve_file // Skip to serving the file
			}
		}

		// Try adding assets/ prefix if it doesn't already have it
		if !strings.HasPrefix(fileName, "assets/") {
			alternativePath := "assets/" + fileName
			log.Printf("ASSET-INFO: Trying alternative path with assets/ prefix: %s", alternativePath)
			attemptedPaths = append(attemptedPaths, strings.Join([]string{branch, runtimeVersion, updateId, alternativePath}, "/"))
			file, err = b.GetFile(branch, runtimeVersion, updateId, alternativePath)
			if err != nil {
				log.Printf("ASSET-ERROR: Alternative path also failed: %v", err)
			} else {
				log.Printf("ASSET-SUCCESS: Found file using alternative path: %s", alternativePath)
				goto serve_file // Skip to serving the file
			}
		}

		// Try adjusting the path for _expo/static/ paths
		if strings.Contains(fileName, "static/js/") {
			parts := strings.Split(fileName, "/")
			if len(parts) > 3 {
				// Try to find just the filename part, ignoring directory structure
				fileNameOnly := parts[len(parts)-1]
				log.Printf("ASSET-INFO: Trying with just the filename: %s", fileNameOnly)
				attemptedPaths = append(attemptedPaths, strings.Join([]string{branch, runtimeVersion, updateId, fileNameOnly}, "/"))
				file, err = b.GetFile(branch, runtimeVersion, updateId, fileNameOnly)
				if err != nil {
					log.Printf("ASSET-ERROR: Alternative path also failed: %v", err)
				} else {
					log.Printf("ASSET-SUCCESS: Found file using just filename: %s", fileNameOnly)
					goto serve_file // Skip to serving the file
				}
			}
		}

		// Try with URL decoding the filename if it contains percent encodings
		if strings.Contains(fileName, "%") {
			decodedPath, decodeErr := url.QueryUnescape(fileName)
			if decodeErr == nil && decodedPath != fileName {
				log.Printf("ASSET-INFO: Trying with URL-decoded path: %s", decodedPath)
				attemptedPaths = append(attemptedPaths, strings.Join([]string{branch, runtimeVersion, updateId, decodedPath}, "/"))
				file, err = b.GetFile(branch, runtimeVersion, updateId, decodedPath)
				if err != nil {
					log.Printf("ASSET-ERROR: URL-decoded path also failed: %v", err)
				} else {
					log.Printf("ASSET-SUCCESS: Found file using URL-decoded path: %s", decodedPath)
					goto serve_file // Skip to serving the file
				}
			}
		}

		// List what files are actually in the update directory to help debugging
		log.Printf("ASSET-CRITICAL: Asset %s not found, checking what files exist in update %s", fileName, updateId)

		// If all attempts failed
		log.Printf("ASSET-CRITICAL: Asset not found after trying %d paths. Request path: %s", len(attemptedPaths), path)
		for i, p := range attemptedPaths {
			log.Printf("ASSET-DEBUG: Attempted path %d: %s", i+1, p)
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	} else {
		log.Printf("ASSET-SUCCESS: Successfully found file using path: %s", fileName)
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
