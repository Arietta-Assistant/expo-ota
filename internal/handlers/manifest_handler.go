package handlers

import (
	"bytes"
	"encoding/json"
	"expo-open-ota/internal/bucket"
	"expo-open-ota/internal/crypto"
	"expo-open-ota/internal/keyStore"
	"expo-open-ota/internal/metrics"
	"expo-open-ota/internal/types"
	"expo-open-ota/internal/update"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func createMultipartResponse(headers map[string][]string, jsonContent interface{}) (*multipart.Writer, *bytes.Buffer, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	field, err := writer.CreatePart(headers)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating multipart field: %w", err)
	}
	contentJSON, err := json.Marshal(jsonContent)
	if err != nil {
		return nil, nil, fmt.Errorf("error marshaling JSON: %w", err)
	}
	if _, err := field.Write(contentJSON); err != nil {
		return nil, nil, fmt.Errorf("error writing JSON content: %w", err)
	}
	return writer, &buf, nil
}

func signDirectiveOrManifest(content interface{}, expectSignatureHeader string) (string, error) {
	if expectSignatureHeader == "" {
		return "", nil
	}
	privateKey := keyStore.GetPrivateExpoKey()
	if privateKey == "" {
		log.Printf("Warning: No private key available for signing. Continuing without signature.")
		return "", nil
	}
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return "", fmt.Errorf("error stringifying content: %w", err)
	}
	signedHash, err := crypto.SignRSASHA256(string(contentJSON), privateKey)
	if err != nil {
		log.Printf("Warning: Error signing content with private key: %v. Continuing without signature.", err)
		return "", nil
	}
	return signedHash, nil
}

func writeResponse(w http.ResponseWriter, writer *multipart.Writer, buf *bytes.Buffer, protocolVersion int64, runtimeVersion string, requestID string) {
	w.Header().Set("expo-protocol-version", strconv.FormatInt(protocolVersion, 10))
	w.Header().Set("expo-sfv-version", "0")
	w.Header().Set("cache-control", "private, max-age=0")
	w.Header().Set("content-type", "multipart/mixed; boundary="+writer.Boundary())
	if err := writer.Close(); err != nil {
		log.Printf("[RequestID: %s] Error closing multipart writer: %v", requestID, err)
		http.Error(w, "Error closing multipart writer", http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("[RequestID: %s] Error writing response: %v", requestID, err)
	}
}

func putResponse(w http.ResponseWriter, r *http.Request, content interface{}, fieldName string, runtimeVersion string, protocolVersion int64, requestID string) {
	signedHash, err := signDirectiveOrManifest(content, r.Header.Get("expo-expect-signature"))
	if err != nil {
		log.Printf("[RequestID: %s] Error signing content: %v", requestID, err)
		http.Error(w, "Error signing content", http.StatusInternalServerError)
		return
	}
	headers := map[string][]string{
		"Content-Disposition": {fmt.Sprintf("form-data; name=\"%s\"", fieldName)},
		"Content-Type":        {"application/json"},
		"content-type":        {"application/json; charset=utf-8"},
	}
	if signedHash != "" {
		headers["expo-signature"] = []string{fmt.Sprintf("sig=\"%s\", keyid=\"main\"", signedHash)}
	}
	writer, buf, err := createMultipartResponse(headers, content)
	if err != nil {
		log.Printf("[RequestID: %s] Error creating multipart response: %v", requestID, err)
		http.Error(w, "Error creating multipart response", http.StatusInternalServerError)
		return
	}
	writeResponse(w, writer, buf, protocolVersion, runtimeVersion, requestID)
}

func putUpdateInResponse(w http.ResponseWriter, r *http.Request, lastUpdate types.Update, platform string, protocolVersion int64, requestID string) {
	currentUpdateId := r.Header.Get("expo-current-update-id")
	log.Printf("[RequestID: %s] Processing update request: updateId=%s, branch=%s, runtimeVersion=%s",
		requestID, lastUpdate.UpdateId, lastUpdate.Branch, lastUpdate.RuntimeVersion)

	metadata, err := update.GetMetadata(lastUpdate)
	if err != nil {
		log.Printf("[RequestID: %s] Error getting metadata: %v", requestID, err)
		http.Error(w, "Error getting metadata", http.StatusInternalServerError)
		return
	}

	// Extract build number from Expo-Extra-Params
	currentBuild := ""
	extraParams := r.Header.Get("Expo-Extra-Params")
	if extraParams != "" {
		// Parse the extra params format: expo-build-number="build-5"
		extraParamsParts := strings.Split(extraParams, ",")
		for _, part := range extraParamsParts {
			part = strings.TrimSpace(part)
			if strings.Contains(part, "expo-build-number") {
				// Extract the value between quotes
				start := strings.Index(part, "\"")
				end := strings.LastIndex(part, "\"")
				if start != -1 && end != -1 && end > start {
					currentBuild = part[start+1 : end]
					log.Printf("[RequestID: %s] Found build number in Expo-Extra-Params: %s", requestID, currentBuild)
				}
				break
			}
		}
	}

	// If we didn't find it in Expo-Extra-Params, try the direct header (fallback)
	if currentBuild == "" {
		currentBuild = r.Header.Get("expo-build-number")
		if currentBuild != "" {
			log.Printf("[RequestID: %s] Using fallback expo-build-number header: %s", requestID, currentBuild)
		}
	}

	// Add debug logging
	log.Printf("[RequestID: %s] Client build number: %s, Current update ID: %s",
		requestID, currentBuild, currentUpdateId)

	// Get build number from update ID
	updateBuild := lastUpdate.UpdateId
	log.Printf("[RequestID: %s] Update ID (containing build number): %s", requestID, updateBuild)

	// Only check builds if we have a current build number
	if currentBuild != "" {
		log.Printf("[RequestID: %s] Comparing client build %s with update %s",
			requestID, currentBuild, updateBuild)

		result := compareBuildNumbersWithRequestID(currentBuild, updateBuild, requestID)
		if result >= 0 {
			log.Printf("[RequestID: %s] No update needed - client build (%s) >= available update (%s)",
				requestID, currentBuild, updateBuild)
			putNoUpdateAvailableInResponse(w, r, lastUpdate.RuntimeVersion, protocolVersion, requestID)
			return
		} else {
			log.Printf("[RequestID: %s] Update needed - client build (%s) < available update (%s)",
				requestID, currentBuild, updateBuild)
		}
	} else {
		log.Printf("[RequestID: %s] Client did not provide build number, skipping build comparison", requestID)
	}

	// Check update ID match only if build number check doesn't apply
	if currentUpdateId != "" && currentUpdateId == crypto.ConvertSHA256HashToUUID(metadata.ID) && protocolVersion == 1 {
		log.Printf("[RequestID: %s] No update needed - client already has update ID %s",
			requestID, currentUpdateId)
		putNoUpdateAvailableInResponse(w, r, lastUpdate.RuntimeVersion, protocolVersion, requestID)
		return
	}

	log.Printf("[RequestID: %s] Sending update to client: ID=%s, Branch=%s, RuntimeVersion=%s",
		requestID, lastUpdate.UpdateId, lastUpdate.Branch, lastUpdate.RuntimeVersion)

	manifest, err := update.ComposeUpdateManifest(&metadata, lastUpdate, platform)
	if err != nil {
		log.Printf("[RequestID: %s] Error composing manifest: %v", requestID, err)
		http.Error(w, "Error composing manifest", http.StatusInternalServerError)
		return
	}

	// Ensure that manifest has all the required fields and structure
	if manifest.LaunchAsset.Key == "" || manifest.LaunchAsset.Url == "" {
		log.Printf("[RequestID: %s] WARNING: LaunchAsset missing key or URL - this may cause updates to fail", requestID)

		// Try to fix missing LaunchAsset URL if needed
		resolvedBucket := bucket.GetBucket()

		// Special handling for JS bundle - try to find any JS file that could be used
		jsFiles := []string{"bundle.js", "index.js", "app.js", "index.bundle", "app.bundle"}

		for _, jsFile := range jsFiles {
			_, err := resolvedBucket.GetFile(lastUpdate.Branch, lastUpdate.RuntimeVersion, lastUpdate.UpdateId, jsFile)
			if err == nil {
				log.Printf("[RequestID: %s] Found potential bundle file: %s", requestID, jsFile)
				// Update the manifest to use this file
				if manifest.LaunchAsset.Key == "" {
					manifest.LaunchAsset.Key = jsFile
				}
				manifest.LaunchAsset.Url = fmt.Sprintf("/assets/%s/%s/%s/%s?platform=%s",
					lastUpdate.Branch, lastUpdate.RuntimeVersion, lastUpdate.UpdateId, jsFile, platform)
				break
			}
		}
	}

	metrics.TrackUpdateDownload(platform, lastUpdate.RuntimeVersion, lastUpdate.Branch, metadata.ID, "update")
	log.Printf("[RequestID: %s] Update download tracked successfully", requestID)

	// Log the complete manifest being sent for debugging purposes
	manifestJSON, _ := json.MarshalIndent(manifest, "", "  ")
	log.Printf("[RequestID: %s] Sending manifest to client: %s", requestID, string(manifestJSON))

	putResponse(w, r, manifest, "manifest", lastUpdate.RuntimeVersion, protocolVersion, requestID)
}

func compareBuildNumbersWithRequestID(current, update string, requestID string) int {
	// Use extractBuildNumber to get build numbers
	currentNum := extractBuildNumber(current)
	updateNum := extractBuildNumber(update)

	// If either build number couldn't be extracted
	if currentNum == -1 || updateNum == -1 {
		// If update build is unknown or couldn't be extracted, assume an update is needed
		if updateNum == -1 {
			log.Printf("[RequestID: %s] Update build number couldn't be extracted, assuming update is needed", requestID)
			return -1
		}

		// If client build is unknown, do string comparison as fallback
		log.Printf("[RequestID: %s] Client build number couldn't be extracted, using string comparison", requestID)
		result := strings.Compare(current, update)

		if result < 0 {
			log.Printf("[RequestID: %s] String compare: client build %s is alphabetically before update %s, update needed",
				requestID, current, update)
		} else if result > 0 {
			log.Printf("[RequestID: %s] String compare: client build %s is alphabetically after update %s, no update needed",
				requestID, current, update)
		} else {
			log.Printf("[RequestID: %s] String compare: client build equals update, no update needed", requestID)
		}

		return result
	}

	// Compare build numbers numerically
	log.Printf("[RequestID: %s] Comparing as numbers: client=%d, update=%d", requestID, currentNum, updateNum)
	if currentNum < updateNum {
		log.Printf("[RequestID: %s] Client build %d is older than update build %d, update needed",
			requestID, currentNum, updateNum)
		return -1 // Client has older build, needs update
	} else if currentNum > updateNum {
		log.Printf("[RequestID: %s] Client build %d is newer than update build %d, no update needed",
			requestID, currentNum, updateNum)
		return 1 // Client has newer build, no update needed
	}

	log.Printf("[RequestID: %s] Client build equals update build, no update needed", requestID)
	return 0 // Same build
}

// extractBuildNumber extracts build number from a string like "build-NUMBER-updateid" or just "12"
// Returns the build number as an integer, or -1 if not found
func extractBuildNumber(str string) int {
	// Only extract from build-NUMBER format
	if strings.HasPrefix(str, "build-") {
		parts := strings.SplitN(strings.TrimPrefix(str, "build-"), "-", 2)
		if len(parts) > 0 {
			num, err := strconv.Atoi(parts[0])
			if err == nil {
				return num
			}
		}
	}

	// For direct number format (less common)
	num, err := strconv.Atoi(str)
	if err == nil {
		return num
	}

	// Not a valid build number format
	return -1
}

func putRollbackInResponse(w http.ResponseWriter, r *http.Request, lastUpdate types.Update, platform string, protocolVersion int64, requestID string) {
	if protocolVersion == 0 {
		http.Error(w, "Rollback not supported in protocol version 0", http.StatusBadRequest)
		return
	}
	embeddedUpdateId := r.Header.Get("expo-embedded-update-id")
	if embeddedUpdateId == "" {
		http.Error(w, "No embedded update id provided", http.StatusBadRequest)
		return
	}
	currentUpdateId := r.Header.Get("expo-current-update-id")
	if currentUpdateId != "" && currentUpdateId == embeddedUpdateId {
		putNoUpdateAvailableInResponse(w, r, lastUpdate.RuntimeVersion, protocolVersion, requestID)
		return
	}
	directive, err := update.CreateRollbackDirective(lastUpdate)
	if err != nil {
		log.Printf("[RequestID: %s] Error creating rollback directive: %v", requestID, err)
		http.Error(w, "Error creating rollback directive", http.StatusInternalServerError)
		return
	}
	metrics.TrackUpdateDownload(platform, lastUpdate.RuntimeVersion, lastUpdate.Branch, lastUpdate.UpdateId, "rollback")
	putResponse(w, r, directive, "directive", lastUpdate.RuntimeVersion, protocolVersion, requestID)
}

func putNoUpdateAvailableInResponse(w http.ResponseWriter, r *http.Request, runtimeVersion string, protocolVersion int64, requestID string) {
	if protocolVersion == 0 {
		http.Error(w, "NoUpdateAvailable directive not available in protocol version 0", http.StatusNoContent)
		return
	}
	directive := update.CreateNoUpdateAvailableDirective()
	putResponse(w, r, directive, "directive", runtimeVersion, protocolVersion, requestID)
}

func ManifestHandler(c *gin.Context) {
	requestID := uuid.New().String()

	// Get required headers
	channelName := c.GetHeader("expo-channel-name")
	protocolVersionStr := c.GetHeader("expo-protocol-version")
	platform := c.GetHeader("expo-platform")
	runtimeVersion := c.GetHeader("expo-runtime-version")
	currentUpdateId := c.GetHeader("expo-current-update-id")

	// Log all headers for debugging
	log.Printf("[RequestID: %s] DEBUG - All headers:", requestID)
	for k, v := range c.Request.Header {
		log.Printf("[RequestID: %s]   %s: %v", requestID, k, v)
	}

	// Extract firebase_token from headers
	firebaseToken := c.GetHeader("firebase_token")

	// If not found in direct headers, try to extract firebase_token from Expo-Extra-Params
	if firebaseToken == "" {
		extraParams := c.GetHeader("Expo-Extra-Params")
		if extraParams != "" {
			log.Printf("[RequestID: %s] Parsing Expo-Extra-Params for firebase_token", requestID)
			extraParamsParts := strings.Split(extraParams, ",")
			for _, part := range extraParamsParts {
				part = strings.TrimSpace(part)
				// Check for both firebase_token and firebase_token formats
				if strings.Contains(part, "firebase_token") || strings.Contains(part, "firebase_token") {
					// Extract the value between quotes
					start := strings.Index(part, "\"")
					end := strings.LastIndex(part, "\"")
					if start != -1 && end != -1 && end > start {
						firebaseToken = part[start+1 : end]
						log.Printf("[RequestID: %s] Found firebase token in Expo-Extra-Params", requestID)
					}
					break
				}
			}
		}
	}

	if firebaseToken != "" {
		log.Printf("[RequestID: %s] Firebase token is present (length: %d)", requestID, len(firebaseToken))
	} else {
		log.Printf("[RequestID: %s] No firebase token found in request", requestID)
	}

	// Extract build number from Expo-Extra-Params
	buildNumber := ""
	extraParams := c.GetHeader("Expo-Extra-Params")
	if extraParams != "" {
		log.Printf("[RequestID: %s] DEBUG - Raw Expo-Extra-Params: %s", requestID, extraParams)

		// Parse the extra params format: expo-build-number="build-5"
		extraParamsParts := strings.Split(extraParams, ",")
		log.Printf("[RequestID: %s] DEBUG - Split into %d parts:", requestID, len(extraParamsParts))
		for i, part := range extraParamsParts {
			part = strings.TrimSpace(part)
			log.Printf("[RequestID: %s]   Part %d: %s", requestID, i, part)

			if strings.Contains(part, "expo-build-number") {
				// Extract the value between quotes
				start := strings.Index(part, "\"")
				end := strings.LastIndex(part, "\"")
				if start != -1 && end != -1 && end > start {
					buildNumber = part[start+1 : end]
					log.Printf("[RequestID: %s] Found build number in Expo-Extra-Params: %s", requestID, buildNumber)
				}
				break
			}
		}
	}

	// If we didn't find it in Expo-Extra-Params, try the direct header (fallback)
	if buildNumber == "" {
		buildNumber = c.GetHeader("expo-build-number")
		if buildNumber != "" {
			log.Printf("[RequestID: %s] Using fallback expo-build-number header: %s", requestID, buildNumber)
		}
	}

	// Log all request details for debugging
	log.Printf("[RequestID: %s] Manifest request received: Path: %s, Headers: channel=%s, platform=%s, runtime=%s, build=%s, currentUpdate=%s",
		requestID,
		c.Request.URL.Path,
		channelName,
		platform,
		runtimeVersion,
		buildNumber,
		currentUpdateId)

	// Add extra debug logging for Expo-Extra-Params
	log.Printf("[RequestID: %s] Expo-Extra-Params: %s, Extracted build number: %s",
		requestID, extraParams, buildNumber)

	// Get path parameters
	branch := c.Param("branch")
	pathRuntimeVersion := c.Param("runtimeVersion")

	log.Printf("[RequestID: %s] Path parameters: branch=%s, runtimeVersion=%s", requestID, branch, pathRuntimeVersion)

	// Important: Check the channel vs. branch values
	log.Printf("[RequestID: %s] CHANNEL-TO-BRANCH MAPPING: Request channel=%s, path branch=%s",
		requestID, channelName, branch)

	// If channel and branch differ, try to log why
	if channelName != branch {
		log.Printf("[RequestID: %s] NOTE: Channel name (%s) differs from branch (%s) - this could cause lookup issues",
			requestID, channelName, branch)
	}

	// If runtimeVersion from path is available but header isn't, use the path version
	if pathRuntimeVersion != "" && runtimeVersion == "" {
		runtimeVersion = pathRuntimeVersion
		log.Printf("[RequestID: %s] Using runtime version from path: %s", requestID, runtimeVersion)
	}

	// Validate headers
	if channelName == "" || protocolVersionStr == "" || platform == "" || runtimeVersion == "" {
		log.Printf("[RequestID: %s] Missing required headers: channel=%s, protocol=%s, platform=%s, runtime=%s",
			requestID, channelName, protocolVersionStr, platform, runtimeVersion)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required headers"})
		return
	}

	protocolVersion, err := strconv.ParseInt(protocolVersionStr, 10, 64)
	if err != nil {
		log.Printf("[RequestID: %s] Invalid protocol version: %v", requestID, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid protocol version"})
		return
	}

	// Get the latest update for this channel and runtime version
	log.Printf("[RequestID: %s] Searching for updates in branch=%s, runtimeVersion=%s, buildNumber=%s",
		requestID, branch, runtimeVersion, buildNumber)
	latestUpdate, err := update.GetLatestUpdateBundlePathForRuntimeVersion(branch, runtimeVersion, buildNumber)
	if err != nil {
		log.Printf("[RequestID: %s] Error getting latest update: %v", requestID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error getting latest update"})
		return
	}

	if latestUpdate == nil {
		log.Printf("[RequestID: %s] No update found for branch %s and runtime version %s",
			requestID, branch, runtimeVersion)
		c.JSON(http.StatusNotFound, gin.H{"error": "No update found"})
		return
	}

	log.Printf("[RequestID: %s] Found latest update: ID=%s", requestID, latestUpdate.UpdateId)

	// Return the update manifest
	putUpdateInResponse(c.Writer, c.Request, *latestUpdate, platform, protocolVersion, requestID)
}

func PutUpdateInResponse(w http.ResponseWriter, branch string, runtimeVersion string, updateId string) {
	requestID := uuid.New().String()
	// Get update but don't store unused variable
	_, err := update.GetUpdate(branch, runtimeVersion, updateId)
	if err != nil {
		log.Printf("[RequestID: %s] Error getting update: %v", requestID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Use bucket.GetBucket() instead of undefined update.GetResolvedBucket()
	resolvedBucket := bucket.GetBucket()
	manifestFilePath, err := resolvedBucket.GetFile(branch, runtimeVersion, updateId, "manifest.json")
	if err != nil {
		log.Printf("[RequestID: %s] Error getting manifest file: %v", requestID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer manifestFilePath.Close()

	data, err := io.ReadAll(manifestFilePath)
	if err != nil {
		log.Printf("[RequestID: %s] Error reading manifest file: %v", requestID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var manifest map[string]interface{}
	err = json.Unmarshal(data, &manifest)
	if err != nil {
		log.Printf("[RequestID: %s] Error unmarshalling manifest: %v", requestID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	buildNumber := "unknown"
	if metadata, ok := manifest["metadata"].(map[string]interface{}); ok {
		if extra, ok := metadata["extra"].(map[string]interface{}); ok {
			if bn, ok := extra["buildNumber"].(string); ok {
				buildNumber = bn
			}
		}
	}

	log.Printf("[RequestID: %s] Serving update %s (build: %s) for %s/%s", requestID, updateId, buildNumber, branch, runtimeVersion)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// Legacy function for backward compatibility
func compareBuildNumbers(current, update string) int {
	return compareBuildNumbersWithRequestID(current, update, "LEGACY")
}
