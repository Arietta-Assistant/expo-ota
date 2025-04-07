package handlers

import (
	"bytes"
	"encoding/json"
	"expo-open-ota/internal/auth"
	"expo-open-ota/internal/branch"
	"expo-open-ota/internal/bucket"
	cache2 "expo-open-ota/internal/cache"
	"expo-open-ota/internal/config"
	"expo-open-ota/internal/helpers"
	"expo-open-ota/internal/services"
	"expo-open-ota/internal/types"
	"expo-open-ota/internal/update"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type FileNamesRequest struct {
	FileNames []string `json:"fileNames"`
}

func MarkUpdateAsUploadedHandler(w http.ResponseWriter, r *http.Request) {
	requestID := uuid.New().String()
	vars := mux.Vars(r)
	branchName := vars["BRANCH"]
	platform := r.URL.Query().Get("platform")
	if platform == "" || (platform != "ios" && platform != "android") {
		log.Printf("[RequestID: %s] Invalid platform: %s", requestID, platform)
		http.Error(w, "Invalid platform", http.StatusBadRequest)
		return
	}
	if branchName == "" {
		log.Printf("[RequestID: %s] No branch provided", requestID)
		http.Error(w, "No branch provided", http.StatusBadRequest)
		return
	}
	err := branch.UpsertBranch(branchName)
	if err != nil {
		log.Printf("[RequestID: %s] Error upserting branch: %v", requestID, err)
		http.Error(w, "Error upserting branch", http.StatusInternalServerError)
		return
	}
	expoAuth := helpers.GetExpoAuth(r)
	expoAccount, err := services.FetchExpoUserAccountInformations(expoAuth)
	if err != nil {
		log.Printf("[RequestID: %s] Error fetching expo account informations: %v", requestID, err)
		http.Error(w, "Error fetching expo account informations", http.StatusUnauthorized)
		return
	}
	if expoAccount == nil {
		log.Printf("[RequestID: %s] No expo account found", requestID)
		http.Error(w, "No expo account found", http.StatusUnauthorized)
		return
	}
	currentExpoUsername := services.FetchSelfExpoUsername()
	if expoAccount.Username != currentExpoUsername {
		log.Printf("[RequestID: %s] Invalid expo account", requestID)
		http.Error(w, "Invalid expo account", http.StatusUnauthorized)
		return
	}
	runtimeVersion := r.URL.Query().Get("runtimeVersion")
	if runtimeVersion == "" {
		log.Printf("[RequestID: %s] No runtime version provided", requestID)
		http.Error(w, "No runtime version provided", http.StatusBadRequest)
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
	if errorVerify != nil {
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
	latestUpdate, err := update.GetLatestUpdateBundlePathForRuntimeVersion(branchName, runtimeVersion)
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

func RequestUploadLocalFileHandler(w http.ResponseWriter, r *http.Request) {
	requestID := uuid.New().String()

	// Verify Firebase token
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		log.Printf("[RequestID: %s] No authorization header provided", requestID)
		http.Error(w, "No authorization header provided", http.StatusUnauthorized)
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	tokenInfo, err := auth.VerifyFirebaseToken(token)
	if err != nil || tokenInfo == nil {
		log.Printf("[RequestID: %s] Invalid Firebase token: %v", requestID, err)
		http.Error(w, "Invalid Firebase token", http.StatusUnauthorized)
		return
	}

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
	if platform == "" || (platform != "ios" && platform != "android") {
		log.Printf("[RequestID: %s] Invalid platform: %s", requestID, platform)
		http.Error(w, "Invalid platform", http.StatusBadRequest)
		return
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
	latestUpdate, err := update.GetLatestUpdateBundlePathForRuntimeVersion(branchName, runtimeVersion)
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

func RequestUploadUrlHandler(w http.ResponseWriter, r *http.Request) {
	requestID := uuid.New().String()

	// Verify Firebase token
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		log.Printf("[RequestID: %s] No authorization header provided", requestID)
		http.Error(w, "No authorization header provided", http.StatusUnauthorized)
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	tokenInfo, err := auth.VerifyFirebaseToken(token)
	if err != nil || tokenInfo == nil {
		log.Printf("[RequestID: %s] Invalid Firebase token: %v", requestID, err)
		http.Error(w, "Invalid Firebase token", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	branchName := vars["BRANCH"]
	if branchName == "" {
		log.Printf("[RequestID: %s] No branch name provided", requestID)
		http.Error(w, "No branch name provided", http.StatusBadRequest)
		return
	}

	platform := r.URL.Query().Get("platform")
	if platform == "" || (platform != "ios" && platform != "android") {
		log.Printf("[RequestID: %s] Invalid platform: %s", requestID, platform)
		http.Error(w, "Invalid platform", http.StatusBadRequest)
		return
	}

	commitHash := r.URL.Query().Get("commitHash")
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

	var request FileNamesRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		log.Printf("[RequestID: %s] Error decoding JSON body: %v", requestID, err)
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if len(request.FileNames) == 0 {
		log.Printf("[RequestID: %s] No file names provided", requestID)
		http.Error(w, "No file names provided", http.StatusBadRequest)
		return
	}

	updateId := time.Now().UnixNano() / int64(time.Millisecond)
	updateRequests, err := bucket.RequestUploadUrlsForFileUpdates(branchName, runtimeVersion, fmt.Sprintf("%d", updateId), request.FileNames)
	if err != nil {
		log.Printf("[RequestID: %s] Error requesting upload urls: %v", requestID, err)
		http.Error(w, "Error requesting upload urls", http.StatusInternalServerError)
		return
	}

	fileUpdateMetadata := map[string]interface{}{
		"platform":       platform,
		"commitHash":     commitHash,
		"buildNumber":    buildNumber,
		"runtimeVersion": runtimeVersion,
	}

	marshalledMetadata, err := json.Marshal(fileUpdateMetadata)
	if err != nil {
		log.Printf("[RequestID: %s] Error marshalling file update metadata: %v", requestID, err)
		http.Error(w, "Error marshalling file update metadata", http.StatusInternalServerError)
		return
	}

	metadataReader := bytes.NewReader(marshalledMetadata)
	resolvedBucket := bucket.GetBucket()
	err = resolvedBucket.UploadFileIntoUpdate(types.Update{
		Branch:         branchName,
		RuntimeVersion: runtimeVersion,
		UpdateId:       fmt.Sprintf("%d", updateId),
		CreatedAt:      time.Duration(updateId) * time.Millisecond,
	}, "update-metadata.json", metadataReader)

	cache := cache2.GetCache()
	cacheKey := update.ComputeLastUpdateCacheKey(branchName, runtimeVersion)
	cache.Delete(cacheKey)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("expo-update-id", fmt.Sprintf("%d", updateId))
	if err := json.NewEncoder(w).Encode(updateRequests); err != nil {
		log.Printf("[RequestID: %s] Error encoding response: %v", requestID, err)
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
	}
	w.WriteHeader(http.StatusOK)
}
