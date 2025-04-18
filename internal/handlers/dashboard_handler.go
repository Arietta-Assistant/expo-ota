package handlers

import (
	"expo-open-ota/config"
	"expo-open-ota/internal/dashboard"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
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

func GetSettingsHandler(c *gin.Context) {
	settings := dashboard.GetDashboardConfig()
	c.JSON(http.StatusOK, settings)
}

func GetBranchesHandler(c *gin.Context) {
	branches, err := dashboard.GetBranches()
	if err != nil {
		log.Printf("Error getting branches: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error getting branches"})
		return
	}
	c.JSON(http.StatusOK, branches)
}

func GetRuntimeVersionsHandler(c *gin.Context) {
	branch := c.Param("branch")
	if branch == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No branch provided"})
		return
	}

	versions, err := dashboard.GetRuntimeVersions(branch)
	if err != nil {
		log.Printf("Error getting runtime versions: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error getting runtime versions"})
		return
	}
	c.JSON(http.StatusOK, versions)
}

func GetUpdatesHandler(c *gin.Context) {
	branch := c.Param("branch")
	runtimeVersion := c.Param("runtimeVersion")

	if branch == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No branch provided"})
		return
	}

	if runtimeVersion == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No runtime version provided"})
		return
	}

	updates, err := dashboard.GetUpdates(branch, runtimeVersion)
	if err != nil {
		log.Printf("Error getting updates: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error getting updates"})
		return
	}
	c.JSON(http.StatusOK, updates)
}
