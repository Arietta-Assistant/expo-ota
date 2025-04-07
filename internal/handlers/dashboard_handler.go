package handlers

import (
	"encoding/json"
	"expo-open-ota/config"
	"expo-open-ota/internal/bucket"
	cache2 "expo-open-ota/internal/cache"
	"expo-open-ota/internal/crypto"
	"expo-open-ota/internal/dashboard"
	"expo-open-ota/internal/services"
	update2 "expo-open-ota/internal/update"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/mux"
)

type BranchMapping struct {
	BranchName     string  `json:"branchName"`
	ReleaseChannel *string `json:"releaseChannel"`
}

type UpdateItem struct {
	UpdateUUID string `json:"updateUUID"`
	UpdateId   string `json:"updateId"`
	CreatedAt  string `json:"createdAt"`
	CommitHash string `json:"commitHash"`
	Platform   string `json:"platform"`
}

type DashboardConfig struct {
	BASE_URL                      string `json:"BASE_URL"`
	EXPO_APP_ID                   string `json:"EXPO_APP_ID"`
	EXPO_ACCESS_TOKEN             string `json:"EXPO_ACCESS_TOKEN"`
	CACHE_MODE                    string `json:"CACHE_MODE"`
	REDIS_HOST                    string `json:"REDIS_HOST"`
	REDIS_PORT                    string `json:"REDIS_PORT"`
	STORAGE_MODE                  string `json:"STORAGE_MODE"`
	S3_BUCKET_NAME                string `json:"S3_BUCKET_NAME"`
	LOCAL_BUCKET_BASE_PATH        string `json:"LOCAL_BUCKET_BASE_PATH"`
	KEYS_STORAGE_TYPE             string `json:"KEYS_STORAGE_TYPE"`
	KEYS_STORAGE_BASE_PATH        string `json:"KEYS_STORAGE_BASE_PATH"`
	KEYS_STORAGE_BUCKET_NAME      string `json:"KEYS_STORAGE_BUCKET_NAME"`
	KEYS_STORAGE_ACCESS_KEY       string `json:"KEYS_STORAGE_ACCESS_KEY"`
	KEYS_STORAGE_SECRET_KEY       string `json:"KEYS_STORAGE_SECRET_KEY"`
	KEYS_STORAGE_ENDPOINT         string `json:"KEYS_STORAGE_ENDPOINT"`
	KEYS_STORAGE_REGION           string `json:"KEYS_STORAGE_REGION"`
	KEYS_STORAGE_FORCE_PATH_STYLE bool   `json:"KEYS_STORAGE_FORCE_PATH_STYLE"`
}

func GetDashboardConfig(c *gin.Context) {
	expoToken := config.GetEnv("EXPO_ACCESS_TOKEN")
	tokenDisplay := ""
	if expoToken != "" {
		tokenDisplay = "***" + expoToken[:5]
	}

	config := DashboardConfig{
		BASE_URL:                      config.GetEnv("BASE_URL"),
		EXPO_APP_ID:                   config.GetEnv("EXPO_APP_ID"),
		EXPO_ACCESS_TOKEN:             tokenDisplay,
		CACHE_MODE:                    config.GetEnv("CACHE_MODE"),
		REDIS_HOST:                    config.GetEnv("REDIS_HOST"),
		REDIS_PORT:                    config.GetEnv("REDIS_PORT"),
		STORAGE_MODE:                  config.GetEnv("STORAGE_MODE"),
		S3_BUCKET_NAME:                config.GetEnv("S3_BUCKET_NAME"),
		LOCAL_BUCKET_BASE_PATH:        config.GetEnv("LOCAL_BUCKET_BASE_PATH"),
		KEYS_STORAGE_TYPE:             config.GetEnv("KEYS_STORAGE_TYPE"),
		KEYS_STORAGE_BASE_PATH:        config.GetEnv("KEYS_STORAGE_BASE_PATH"),
		KEYS_STORAGE_BUCKET_NAME:      config.GetEnv("KEYS_STORAGE_BUCKET_NAME"),
		KEYS_STORAGE_ACCESS_KEY:       config.GetEnv("KEYS_STORAGE_ACCESS_KEY"),
		KEYS_STORAGE_SECRET_KEY:       config.GetEnv("KEYS_STORAGE_SECRET_KEY"),
		KEYS_STORAGE_ENDPOINT:         config.GetEnv("KEYS_STORAGE_ENDPOINT"),
		KEYS_STORAGE_REGION:           config.GetEnv("KEYS_STORAGE_REGION"),
		KEYS_STORAGE_FORCE_PATH_STYLE: config.GetEnv("KEYS_STORAGE_FORCE_PATH_STYLE") == "true",
	}

	c.JSON(200, config)
}

func GetSettingsHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve all in config.GetEnv & return as JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	expoToken := config.GetEnv("EXPO_ACCESS_TOKEN")
	tokenDisplay := ""
	if expoToken != "" {
		tokenDisplay = "***" + expoToken[:5]
	}

	settings := DashboardConfig{
		BASE_URL:                      config.GetEnv("BASE_URL"),
		EXPO_APP_ID:                   config.GetEnv("EXPO_APP_ID"),
		EXPO_ACCESS_TOKEN:             tokenDisplay,
		CACHE_MODE:                    config.GetEnv("CACHE_MODE"),
		REDIS_HOST:                    config.GetEnv("REDIS_HOST"),
		REDIS_PORT:                    config.GetEnv("REDIS_PORT"),
		STORAGE_MODE:                  config.GetEnv("STORAGE_MODE"),
		S3_BUCKET_NAME:                config.GetEnv("S3_BUCKET_NAME"),
		LOCAL_BUCKET_BASE_PATH:        config.GetEnv("LOCAL_BUCKET_BASE_PATH"),
		KEYS_STORAGE_TYPE:             config.GetEnv("KEYS_STORAGE_TYPE"),
		KEYS_STORAGE_BASE_PATH:        config.GetEnv("KEYS_STORAGE_BASE_PATH"),
		KEYS_STORAGE_BUCKET_NAME:      config.GetEnv("KEYS_STORAGE_BUCKET_NAME"),
		KEYS_STORAGE_ACCESS_KEY:       config.GetEnv("KEYS_STORAGE_ACCESS_KEY"),
		KEYS_STORAGE_SECRET_KEY:       config.GetEnv("KEYS_STORAGE_SECRET_KEY"),
		KEYS_STORAGE_ENDPOINT:         config.GetEnv("KEYS_STORAGE_ENDPOINT"),
		KEYS_STORAGE_REGION:           config.GetEnv("KEYS_STORAGE_REGION"),
		KEYS_STORAGE_FORCE_PATH_STYLE: config.GetEnv("KEYS_STORAGE_FORCE_PATH_STYLE") == "true",
	}

	json.NewEncoder(w).Encode(settings)
}

func GetBranchesHandler(w http.ResponseWriter, r *http.Request) {
	resolvedBucket := bucket.GetBucket()
	branches, err := resolvedBucket.GetBranches()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	cacheKey := dashboard.ComputeGetBranchesCacheKey()
	cache := cache2.GetCache()
	if cacheValue := cache.Get(cacheKey); cacheValue != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		var branches []BranchMapping
		json.Unmarshal([]byte(cacheValue), &branches)
		json.NewEncoder(w).Encode(branches)
		return
	}
	branchesMapping, err := services.FetchExpoBranchesMapping()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var response []BranchMapping
	for _, branch := range branches {
		var releaseChannel *string
		for _, mapping := range branchesMapping {
			if mapping.BranchName == branch {
				releaseChannel = &mapping.ChannelName
				break
			}
		}
		response = append(response, BranchMapping{
			BranchName:     branch,
			ReleaseChannel: releaseChannel,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
	marshaledResponse, _ := json.Marshal(response)
	cache.Set(cacheKey, string(marshaledResponse), nil)
}

func GetRuntimeVersionsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	branchName := vars["BRANCH"]
	cacheKey := dashboard.ComputeGetRuntimeVersionsCacheKey(branchName)
	cache := cache2.GetCache()
	if cacheValue := cache.Get(cacheKey); cacheValue != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		var runtimeVersions []bucket.RuntimeVersionWithStats
		json.Unmarshal([]byte(cacheValue), &runtimeVersions)
		json.NewEncoder(w).Encode(runtimeVersions)
		return
	}
	resolvedBucket := bucket.GetBucket()
	runtimeVersions, err := resolvedBucket.GetRuntimeVersions(branchName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	sort.Slice(runtimeVersions, func(i, j int) bool {
		timeI, _ := time.Parse(time.RFC3339, runtimeVersions[i].CreatedAt)
		timeJ, _ := time.Parse(time.RFC3339, runtimeVersions[j].CreatedAt)
		return timeI.After(timeJ)
	})
	json.NewEncoder(w).Encode(runtimeVersions)
	marshaledResponse, _ := json.Marshal(runtimeVersions)
	cache.Set(cacheKey, string(marshaledResponse), nil)
}

func GetUpdatesHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	branchName := vars["BRANCH"]
	runtimeVersion := vars["RUNTIME_VERSION"]
	cacheKey := dashboard.ComputeGetUpdatesCacheKey(branchName, runtimeVersion)
	cache := cache2.GetCache()
	if cacheValue := cache.Get(cacheKey); cacheValue != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		var updatesResponse []UpdateItem
		json.Unmarshal([]byte(cacheValue), &updatesResponse)
		json.NewEncoder(w).Encode(updatesResponse)
		return
	}
	resolvedBucket := bucket.GetBucket()
	updates, err := resolvedBucket.GetUpdates(branchName, runtimeVersion)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var updatesResponse []UpdateItem
	for _, update := range updates {
		isValid := update2.IsUpdateValid(update)
		if !isValid {
			continue
		}
		metadata, err := update2.GetMetadata(update)
		if err != nil {
			continue
		}
		numberUpdate, _ := strconv.ParseInt(update.UpdateId, 10, 64)
		commitHash, platform, _ := update2.RetrieveUpdateCommitHashAndPlatform(update)
		updatesResponse = append(updatesResponse, UpdateItem{
			UpdateUUID: crypto.ConvertSHA256HashToUUID(metadata.ID),
			UpdateId:   update.UpdateId,
			CreatedAt:  time.UnixMilli(numberUpdate).UTC().Format(time.RFC3339),
			CommitHash: commitHash,
			Platform:   platform,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	sort.Slice(updatesResponse, func(i, j int) bool {
		timeI, _ := time.Parse(time.RFC3339, updatesResponse[i].CreatedAt)
		timeJ, _ := time.Parse(time.RFC3339, updatesResponse[j].CreatedAt)
		return timeI.After(timeJ)
	})
	json.NewEncoder(w).Encode(updatesResponse)
	marshaledResponse, _ := json.Marshal(updatesResponse)
	cache.Set(cacheKey, string(marshaledResponse), nil)
}
