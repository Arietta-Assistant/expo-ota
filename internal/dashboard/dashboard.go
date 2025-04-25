package dashboard

import (
	"expo-open-ota/config"
	"expo-open-ota/internal/bucket"
	"expo-open-ota/internal/types"
	"log"
	"regexp"
	"sort"
	"strconv"
	"time"
)

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

func GetDashboardConfig() DashboardConfig {
	expoToken := config.GetEnv("EXPO_ACCESS_TOKEN")
	tokenDisplay := ""
	if expoToken != "" {
		tokenDisplay = "***" + expoToken[:5]
	}

	return DashboardConfig{
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
}

type Branch struct {
	BranchName     string `json:"branchName"`
	ReleaseChannel string `json:"releaseChannel,omitempty"`
}

func GetBranches() ([]Branch, error) {
	resolvedBucket := bucket.GetBucket()
	branchNames, err := resolvedBucket.GetBranches()
	if err != nil {
		return nil, err
	}

	// Convert string array to array of Branch objects
	branches := make([]Branch, len(branchNames))
	for i, name := range branchNames {
		branches[i] = Branch{
			BranchName: name,
			// ReleaseChannel is empty by default
		}
	}

	return branches, nil
}

func GetRuntimeVersions(branch string) ([]bucket.RuntimeVersionWithStats, error) {
	resolvedBucket := bucket.GetBucket()
	versions, err := resolvedBucket.GetRuntimeVersions(branch)
	if err != nil {
		return nil, err
	}

	// Sort by last updated time
	sort.Slice(versions, func(i, j int) bool {
		timeI, _ := time.Parse(time.RFC3339, versions[i].LastUpdatedAt)
		timeJ, _ := time.Parse(time.RFC3339, versions[j].LastUpdatedAt)
		return timeI.After(timeJ)
	})

	return versions, nil
}

// ExtractBuildNumber extracts build number from update ID (e.g., "build-11-xxx" → 11)
func ExtractBuildNumber(updateId string) int {
	// Try to match build-NUMBER pattern
	re := regexp.MustCompile(`build-(\d+)`)
	matches := re.FindStringSubmatch(updateId)
	if len(matches) > 1 {
		if num, err := strconv.Atoi(matches[1]); err == nil {
			return num
		}
	}

	// Fallback to any number in the ID
	re = regexp.MustCompile(`\d+`)
	matches = re.FindStringSubmatch(updateId)
	if len(matches) > 0 {
		if num, err := strconv.Atoi(matches[0]); err == nil {
			return num
		}
	}

	return -1
}

func GetUpdates(branch, runtimeVersion string) ([]types.Update, error) {
	resolvedBucket := bucket.GetBucket()
	updates, err := resolvedBucket.GetUpdates(branch, runtimeVersion)
	if err != nil {
		return nil, err
	}

	// Enhance updates with build number information
	for i := range updates {
		buildNum := ExtractBuildNumber(updates[i].UpdateId)
		if buildNum > 0 {
			updates[i].BuildNumber = strconv.Itoa(buildNum)
		}
	}

	// Sort updates by creation time (newest first)
	sort.Slice(updates, func(i, j int) bool {
		// First sort by build number if available
		if updates[i].BuildNumber != "" && updates[j].BuildNumber != "" {
			iBuildNum, iErr := strconv.Atoi(updates[i].BuildNumber)
			jBuildNum, jErr := strconv.Atoi(updates[j].BuildNumber)
			if iErr == nil && jErr == nil {
				return iBuildNum > jBuildNum
			}
		}
		// Fall back to creation time
		return updates[i].CreatedAt > updates[j].CreatedAt
	})

	log.Printf("Found %d updates for branch=%s, runtimeVersion=%s",
		len(updates), branch, runtimeVersion)

	return updates, nil
}

func IsDashboardEnabled() bool {
	return config.GetEnv("USE_DASHBOARD") == "true"
}

func ComputeGetBranchesCacheKey() string {
	return "dashboard:request:getBranches"
}

func ComputeGetRuntimeVersionsCacheKey(branch string) string {
	return "dashboard:request:getRuntimeVersions:" + branch
}

func ComputeGetUpdatesCacheKey(branch string, runtimeVersion string) string {
	return "dashboard:request:getUpdates:" + branch + ":" + runtimeVersion
}
