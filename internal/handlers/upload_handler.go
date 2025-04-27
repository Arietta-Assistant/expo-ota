package handlers

import (
	"encoding/json"
	"expo-open-ota/internal/auth"
	"expo-open-ota/internal/bucket"
	"expo-open-ota/internal/config"
	"expo-open-ota/internal/types"
	"expo-open-ota/internal/update"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type FileNamesRequest struct {
	FileNames []string `json:"fileNames"`
}

func UploadHandler(c *gin.Context) {
	requestID := uuid.New().String()
	branchName := c.Param("BRANCH")
	platform := c.Query("platform")

	if platform == "" || (platform != "ios" && platform != "android" && platform != "all") {
		log.Printf("[RequestID: %s] Invalid platform: %s", requestID, platform)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid platform"})
		return
	}

	if branchName == "" {
		log.Printf("[RequestID: %s] No branch provided", requestID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "No branch provided"})
		return
	}

	// Check for Firebase token if present (making verification optional)
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		decodedToken, err := auth.VerifyFirebaseToken(token)
		if err != nil {
			log.Printf("[RequestID: %s] Invalid Firebase token: %v", requestID, err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Firebase token"})
			return
		}

		// Log user access if token is provided
		log.Printf("[RequestID: %s] User %s (%s) uploading update for branch %s",
			requestID,
			decodedToken.UID,
			decodedToken.Claims["email"],
			branchName)
	}

	// If platform is "all", we'll use "ios" as the default for storage
	if platform == "all" {
		log.Printf("[RequestID: %s] Platform 'all' specified, using 'ios' as the primary platform", requestID)
		platform = "ios"
	}

	// Process the upload
	// ... rest of the upload logic ...
}

func RequestUploadLocalFileHandler(w http.ResponseWriter, r *http.Request) {
	requestID := uuid.New().String()

	// Check for Firebase token if present (making verification optional)
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		tokenInfo, err := auth.VerifyFirebaseToken(token)
		if err != nil || tokenInfo == nil {
			log.Printf("[RequestID: %s] Invalid Firebase token: %v", requestID, err)
			http.Error(w, "Invalid Firebase token", http.StatusUnauthorized)
			return
		}
	}
	// No token verification needed if no auth header provided - similar to RequestUploadUrlHandler

	// Check if we're using a local bucket
	bucketType := config.GetEnv("BUCKET_TYPE")
	if bucketType != string(bucket.LocalBucketType) {
		log.Printf("Invalid bucket type: %s", bucketType)
		http.Error(w, "Invalid bucket type", http.StatusInternalServerError)
		return
	}

	branchName := r.URL.Query().Get("branch")
	if branchName == "" {
		log.Printf("[RequestID: %s] No branch name provided", requestID)
		http.Error(w, "No branch name provided", http.StatusBadRequest)
		return
	}
	platform := r.URL.Query().Get("platform")
	if platform == "" || (platform != "ios" && platform != "android" && platform != "all") {
		log.Printf("[RequestID: %s] Invalid platform: %s", requestID, platform)
		http.Error(w, "Invalid platform", http.StatusBadRequest)
		return
	}

	// If platform is "all", we'll use "ios" as the default for storage
	if platform == "all" {
		log.Printf("[RequestID: %s] Platform 'all' specified, using 'ios' as the primary platform", requestID)
		platform = "ios"
	}

	runtimeVersion := r.URL.Query().Get("runtimeVersion")
	if runtimeVersion == "" {
		log.Printf("[RequestID: %s] No runtime version provided", requestID)
		http.Error(w, "No runtime version provided", http.StatusBadRequest)
		return
	}
	buildNumber := r.URL.Query().Get("buildNumber")
	if buildNumber == "" {
		log.Printf("[RequestID: %s] No build number provided", requestID)
		http.Error(w, "No build number provided", http.StatusBadRequest)
		return
	}
	updateId := r.URL.Query().Get("updateId")
	if updateId == "" {
		log.Printf("[RequestID: %s] No update id provided", requestID)
		http.Error(w, "No update id provided", http.StatusBadRequest)
		return
	}
	currentUpdate, err := update.GetUpdate(branchName, runtimeVersion, updateId)
	if err != nil {
		log.Printf("[RequestID: %s] Error getting update: %v", requestID, err)
		http.Error(w, "Error getting update", http.StatusInternalServerError)
		return
	}
	resolvedBucket := bucket.GetBucket()
	errorVerify := update.VerifyUploadedUpdate(*currentUpdate)
	if err != nil {
		// Delete folder and throw error
		log.Printf("[RequestID: %s] Invalid update, deleting folder...", requestID)
		err := resolvedBucket.DeleteUpdateFolder(branchName, runtimeVersion, updateId)
		if err != nil {
			log.Printf("[RequestID: %s] Error deleting update folder: %v", requestID, err)
			http.Error(w, "Error deleting update folder", http.StatusInternalServerError)
			return
		}
		log.Printf("[RequestID: %s] Invalid update, folder deleted", requestID)
		http.Error(w, fmt.Sprintf("Invalid update %s", errorVerify), http.StatusBadRequest)
		return
	}
	// Now we have to retrieve the latest update and compare hash changes
	latestUpdate, err := update.GetLatestUpdateBundlePathForRuntimeVersion(branchName, runtimeVersion, buildNumber)
	if err != nil || latestUpdate == nil {
		err = update.MarkUpdateAsChecked(*currentUpdate)
		if err != nil {
			log.Printf("[RequestID: %s] Error marking update as checked: %v", requestID, err)
			http.Error(w, "Error marking update as checked", http.StatusInternalServerError)
			return
		}
		log.Printf("[RequestID: %s] No latest update found, update marked as checked", requestID)
		w.WriteHeader(http.StatusOK)
		return
	}
	areUpdatesIdentical, err := update.AreUpdatesIdentical(*currentUpdate, *latestUpdate, platform)
	if err != nil {
		log.Printf("[RequestID: %s] Error comparing updates: %v", requestID, err)
		http.Error(w, "Error comparing updates", http.StatusInternalServerError)
		return
	}
	if !areUpdatesIdentical {
		err = update.MarkUpdateAsChecked(*currentUpdate)
		if err != nil {
			log.Printf("[RequestID: %s] Error marking update as checked: %v", requestID, err)
			http.Error(w, "Error marking update as checked", http.StatusInternalServerError)
			return
		}
		log.Printf("[RequestID: %s] Updates are not identical, update marked as checked", requestID)
		w.WriteHeader(http.StatusOK)
		return
	}
	log.Printf("[RequestID: %s] Updates are identical, delete folder...", requestID)
	err = resolvedBucket.DeleteUpdateFolder(branchName, runtimeVersion, currentUpdate.UpdateId)
	if err != nil {
		log.Printf("[RequestID: %s] Error deleting update folder: %v", requestID, err)
		http.Error(w, "Error deleting update folder", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNotAcceptable)
}

func RequestUploadUrlHandler(c *gin.Context) {
	requestID := uuid.New().String()

	// Check for Firebase token if present
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		_, err := auth.VerifyFirebaseToken(token)
		if err != nil {
			log.Printf("[RequestID: %s] Invalid Firebase token: %v", requestID, err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Firebase token"})
			return
		}
	}

	branchName := c.Param("branch")
	if branchName == "" {
		log.Printf("[RequestID: %s] No branch name provided", requestID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "No branch name provided"})
		return
	}

	// Check for channel override in headers
	channel := c.GetHeader("expo-channel")
	if channel == "" {
		channel = c.GetHeader("expo-extra-params")
	}
	if channel != "" {
		branchName = channel
	}

	platform := c.Query("platform")
	if platform == "" || (platform != "ios" && platform != "android" && platform != "all") {
		log.Printf("[RequestID: %s] Invalid platform: %s", requestID, platform)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid platform"})
		return
	}

	// If platform is "all", we'll use "ios" as the default for storage
	// The client will upload both iOS and Android assets
	if platform == "all" {
		log.Printf("[RequestID: %s] Platform 'all' specified, using 'ios' as the primary platform for metadata", requestID)
		platform = "ios"
	}

	commitHash := c.Query("commitHash")
	runtimeVersion := c.Query("runtimeVersion")
	if runtimeVersion == "" {
		log.Printf("[RequestID: %s] No runtime version provided", requestID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "No runtime version provided"})
		return
	}

	buildNumber := c.Query("buildNumber")
	customUpdateId := c.Query("updateId") // Check for custom update ID

	if buildNumber == "" {
		// Try to get build number from expo-extra-params
		extraParams := c.GetHeader("expo-extra-params")
		if extraParams != "" {
			// Parse the extra params JSON
			var extra map[string]interface{}
			if err := json.Unmarshal([]byte(extraParams), &extra); err == nil {
				if updateCode, ok := extra["updateCode"].(string); ok {
					buildNumber = updateCode
					log.Printf("[RequestID: %s] Found build number in expo-extra-params: %s", requestID, buildNumber)
				}
			} else {
				// Try parsing as comma-separated string
				extraParamsParts := strings.Split(extraParams, ",")
				for _, part := range extraParamsParts {
					part = strings.TrimSpace(part)
					if strings.Contains(part, "expo-build-number") {
						// Extract the value between quotes
						start := strings.Index(part, "\"")
						end := strings.LastIndex(part, "\"")
						if start != -1 && end != -1 && end > start {
							buildNumber = part[start+1 : end]
							log.Printf("[RequestID: %s] Found build number in expo-extra-params string: %s", requestID, buildNumber)
							break
						}
					}
				}
			}
		}
	}

	if buildNumber == "" {
		// If still not found, try the header
		buildNumber = c.GetHeader("expo-build-number")
		if buildNumber != "" {
			log.Printf("[RequestID: %s] Found build number in expo-build-number header: %s", requestID, buildNumber)
		}
	}

	var request FileNamesRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		log.Printf("[RequestID: %s] Error decoding JSON body: %v", requestID, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
		return
	}

	if len(request.FileNames) == 0 {
		log.Printf("[RequestID: %s] No file names provided", requestID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file names provided"})
		return
	}

	// Generate update ID
	var updateId string
	if customUpdateId != "" {
		// Use the custom update ID if provided
		updateId = customUpdateId
		log.Printf("[RequestID: %s] Using custom update ID: %s", requestID, updateId)
	} else if buildNumber != "" {
		// Include build number in update ID if available
		updateId = fmt.Sprintf("build-%s-%s", buildNumber, uuid.New().String())
		log.Printf("[RequestID: %s] Generated update ID with build number: %s", requestID, updateId)
	} else {
		// Default to standard UUID
		updateId = uuid.New().String()
		log.Printf("[RequestID: %s] Generated standard update ID: %s", requestID, updateId)
	}

	// Request upload URLs
	resolvedBucket := bucket.GetBucket()
	requests, err := resolvedBucket.RequestUploadUrlsForFileUpdates(branchName, runtimeVersion, updateId, request.FileNames)
	if err != nil {
		log.Printf("[RequestID: %s] Error requesting upload URLs: %v", requestID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error requesting upload URLs"})
		return
	}

	// Create update record
	newUpdate := types.Update{
		Branch:         branchName,
		RuntimeVersion: runtimeVersion,
		UpdateId:       updateId,
		CommitHash:     commitHash,
		BuildNumber:    buildNumber,
		Platform:       platform,
		CreatedAt:      time.Duration(time.Now().UnixNano()),
	}

	err = update.CreateUpdate(newUpdate)
	if err != nil {
		log.Printf("[RequestID: %s] Error creating update record: %v", requestID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error creating update record"})
		return
	}

	// Check if we have any URLs
	if len(requests) == 0 {
		log.Printf("[RequestID: %s] No URLs generated", requestID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No upload URLs generated"})
		return
	}

	// Create an array of upload requests in the format expected by the client
	uploadRequests := make([]map[string]string, 0, len(requests))

	for _, req := range requests {
		fileName := strings.TrimPrefix(req.Path, fmt.Sprintf("updates/%s/%s/%s/", branchName, runtimeVersion, updateId))
		uploadRequests = append(uploadRequests, map[string]string{
			"requestUploadUrl": req.Url,
			"fileName":         fileName,
			"filePath":         req.Path,
		})
	}

	response := map[string]interface{}{
		"updateId":       updateId,
		"buildNumber":    buildNumber, // Include build number in response
		"uploadRequests": uploadRequests,
	}

	// Log the response for debugging
	responseJSON, _ := json.Marshal(response)
	log.Printf("[RequestID: %s] Response body: %s", requestID, string(responseJSON))

	c.JSON(http.StatusOK, response)
}
