package bucket

import (
	"encoding/json"
	"errors"
	"expo-open-ota/config"
	"expo-open-ota/internal/services"
	"expo-open-ota/internal/types"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type LocalBucket struct {
	BasePath string
}

func NewLocalBucket() *LocalBucket {
	basePath := config.GetEnv("LOCAL_BUCKET_BASE_PATH")
	if basePath == "" {
		basePath = "./updates"
		log.Printf("LOCAL_BUCKET_BASE_PATH not set, using default: %s", basePath)
	}

	// Ensure the directory exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		log.Printf("Warning: Could not create local bucket directory %s: %v", basePath, err)
	} else {
		log.Printf("Local bucket directory ensured at: %s", basePath)
	}

	return &LocalBucket{BasePath: basePath}
}

func (b *LocalBucket) DeleteUpdateFolder(branch string, runtimeVersion string, updateId string) error {
	if b.BasePath == "" {
		return errors.New("BasePath not set")
	}
	dirPath := filepath.Join(b.BasePath, branch, runtimeVersion, updateId)
	return os.RemoveAll(dirPath)
}

func (b *LocalBucket) RequestUploadUrlForFileUpdate(branch string, runtimeVersion string, updateId string, fileName string) (string, error) {
	if b.BasePath == "" {
		return "", errors.New("BasePath not set")
	}
	dirPath := filepath.Join(b.BasePath, branch, runtimeVersion, updateId)
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return "", err
	}
	token, err := services.GenerateJWTToken(config.GetEnv("JWT_SECRET"), jwt.MapClaims{
		"sub":      services.FetchSelfExpoUsername(),
		"exp":      time.Now().Add(time.Minute * 10).Unix(),
		"filePath": filepath.Join(dirPath, fileName),
		"action":   "uploadLocalFile",
	})
	if err != nil {
		return "", err
	}
	parsedURL, err := url.Parse(config.GetEnv("BASE_URL"))
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}
	parsedURL.Path, err = url.JoinPath(parsedURL.Path, "uploadLocalFile")
	if err != nil {
		return "", fmt.Errorf("error joining path: %w", err)
	}
	query := url.Values{}
	query.Set("token", token)
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

func (b *LocalBucket) GetUpdates(branch string, runtimeVersion string) ([]types.Update, error) {
	if b.BasePath == "" {
		return nil, errors.New("BasePath not set")
	}
	dirPath := filepath.Join(b.BasePath, branch, runtimeVersion)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return []types.Update{}, nil
	}
	var updates []types.Update
	for _, entry := range entries {
		if entry.IsDir() {
			updateId, err := strconv.ParseInt(entry.Name(), 10, 64)
			if err == nil {
				updates = append(updates, types.Update{
					Branch:         branch,
					RuntimeVersion: runtimeVersion,
					UpdateId:       strconv.FormatInt(updateId, 10),
					CreatedAt:      time.Duration(updateId) * time.Millisecond,
				})
			}
		}
	}
	return updates, nil
}

func (b *LocalBucket) GetFile(branch string, runtimeVersion string, updateId string, fileName string) (io.ReadCloser, error) {
	if b.BasePath == "" {
		return nil, errors.New("BasePath not set")
	}

	filePath := filepath.Join(b.BasePath, branch, runtimeVersion, updateId, fileName)

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (b *LocalBucket) GetBranches() ([]string, error) {
	if b.BasePath == "" {
		return nil, errors.New("BasePath not set")
	}

	// Ensure directory exists
	if err := os.MkdirAll(b.BasePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create bucket directory: %w", err)
	}

	entries, err := os.ReadDir(b.BasePath)
	if err != nil {
		return nil, err
	}

	var branches []string
	for _, entry := range entries {
		if entry.IsDir() {
			branches = append(branches, entry.Name())
		}
	}

	// If no branches, create a demo branch
	if len(branches) == 0 {
		log.Printf("No branches found, creating demo branch")

		// Create default demo branch
		demoBranch := "main"
		demoBranchPath := filepath.Join(b.BasePath, demoBranch)

		// Create runtime version directory
		demoRuntimeVersion := "1.0.0"
		demoRuntimePath := filepath.Join(demoBranchPath, demoRuntimeVersion)

		// Create directories
		if err := os.MkdirAll(demoRuntimePath, 0755); err != nil {
			log.Printf("Warning: Failed to create demo branch structure: %v", err)
		} else {
			log.Printf("Created demo branch: %s with runtime version: %s", demoBranch, demoRuntimeVersion)
			branches = append(branches, demoBranch)
		}
	}

	return branches, nil
}

func (b *LocalBucket) GetRuntimeVersions(branch string) ([]RuntimeVersionWithStats, error) {
	if b.BasePath == "" {
		return nil, errors.New("BasePath not set")
	}

	// Ensure branch directory exists
	branchPath := filepath.Join(b.BasePath, branch)
	if err := os.MkdirAll(branchPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create branch directory: %w", err)
	}

	entries, err := os.ReadDir(branchPath)
	if err != nil {
		return nil, err
	}

	var runtimeVersions []RuntimeVersionWithStats
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runtimeVersion := entry.Name()
		updatesPath := filepath.Join(branchPath, runtimeVersion)
		updates, err := os.ReadDir(updatesPath)
		if err != nil {
			continue
		}
		var updateTimestamps []int64
		for _, update := range updates {
			if !update.IsDir() {
				continue
			}
			timestamp, err := strconv.ParseInt(update.Name(), 10, 64)
			if err != nil {
				continue
			}
			updateTimestamps = append(updateTimestamps, timestamp)
		}

		// If no updates, create a sample update for this runtime version
		if len(updateTimestamps) == 0 {
			log.Printf("No updates found for %s/%s, creating a sample update", branch, runtimeVersion)

			// Create a timestamp-based update ID
			now := time.Now().UnixMilli()
			updateDirPath := filepath.Join(updatesPath, strconv.FormatInt(now, 10))

			// Create update directory and add a sample metadata file
			if err := os.MkdirAll(updateDirPath, 0755); err != nil {
				log.Printf("Warning: Failed to create sample update: %v", err)
			} else {
				// Create a simple metadata.json file
				metadataPath := filepath.Join(updateDirPath, "metadata.json")
				sampleMetadata := `{"version":0,"bundler":"metro","fileMetadata":{"android":{"bundle":"","assets":[]},"ios":{"bundle":"","assets":[]}},"extra":{"commitHash":"sample","updateCode":"build-1","platform":"android"}}`

				if err := os.WriteFile(metadataPath, []byte(sampleMetadata), 0644); err != nil {
					log.Printf("Warning: Failed to write sample metadata: %v", err)
				} else {
					log.Printf("Created sample update in %s", updateDirPath)
					updateTimestamps = append(updateTimestamps, now)
				}
			}
		}

		if len(updateTimestamps) == 0 {
			continue
		}

		sort.Slice(updateTimestamps, func(i, j int) bool { return updateTimestamps[i] < updateTimestamps[j] })

		runtimeVersions = append(runtimeVersions, RuntimeVersionWithStats{
			RuntimeVersion:  runtimeVersion,
			CreatedAt:       time.UnixMilli(updateTimestamps[0]).UTC().Format(time.RFC3339),
			LastUpdatedAt:   time.UnixMilli(updateTimestamps[len(updateTimestamps)-1]).UTC().Format(time.RFC3339),
			NumberOfUpdates: len(updateTimestamps),
		})
	}

	// If no runtime versions found, create a default one
	if len(runtimeVersions) == 0 {
		// Create default runtime version
		defaultRuntime := "1.0.0"
		defaultRuntimePath := filepath.Join(branchPath, defaultRuntime)

		if err := os.MkdirAll(defaultRuntimePath, 0755); err != nil {
			log.Printf("Warning: Failed to create default runtime version: %v", err)
		} else {
			// Create a sample update
			now := time.Now().UnixMilli()
			updateDirPath := filepath.Join(defaultRuntimePath, strconv.FormatInt(now, 10))

			if err := os.MkdirAll(updateDirPath, 0755); err != nil {
				log.Printf("Warning: Failed to create sample update: %v", err)
			} else {
				// Create a simple metadata.json file
				metadataPath := filepath.Join(updateDirPath, "metadata.json")
				sampleMetadata := `{"version":0,"bundler":"metro","fileMetadata":{"android":{"bundle":"","assets":[]},"ios":{"bundle":"","assets":[]}},"extra":{"commitHash":"sample","updateCode":"build-1","platform":"android"}}`

				if err := os.WriteFile(metadataPath, []byte(sampleMetadata), 0644); err != nil {
					log.Printf("Warning: Failed to write sample metadata: %v", err)
				} else {
					log.Printf("Created sample update in %s", updateDirPath)

					// Add to runtime versions
					runtimeVersions = append(runtimeVersions, RuntimeVersionWithStats{
						RuntimeVersion:  defaultRuntime,
						CreatedAt:       time.Now().UTC().Format(time.RFC3339),
						LastUpdatedAt:   time.Now().UTC().Format(time.RFC3339),
						NumberOfUpdates: 1,
					})
				}
			}
		}
	}

	return runtimeVersions, nil
}

func (b *LocalBucket) UploadFileIntoUpdate(update types.Update, fileName string, file io.Reader) error {
	// Create the full path preserving any directory structure in fileName
	filePath := filepath.Join(b.BasePath, update.Branch, update.RuntimeVersion, update.UpdateId, fileName)

	// Ensure all parent directories exist
	err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create directory structure for %s: %w", fileName, err)
	}

	out, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", fileName, err)
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", fileName, err)
	}

	log.Printf("Successfully uploaded file to %s", filePath)
	return nil
}

func ValidateUploadTokenAndResolveFilePath(token string) (string, error) {
	claims := jwt.MapClaims{}
	decodedToken, err := services.DecodeAndExtractJWTToken(config.GetEnv("JWT_SECRET"), token, claims)
	if err != nil {
		return "", err
	}
	if !decodedToken.Valid {
		return "", errors.New("invalid token")
	}
	action := claims["action"].(string)
	filePath := claims["filePath"].(string)
	sub := claims["sub"].(string)
	if sub != services.FetchSelfExpoUsername() {
		return "", errors.New("invalid token sub")
	}
	if action != "uploadLocalFile" {
		return "", errors.New("invalid token action")
	}
	return filePath, nil
}

func HandleUploadFile(filePath string, body multipart.File) (bool, error) {
	err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
	if err != nil {
		return false, err
	}
	file, err := os.Create(filePath)
	if err != nil {
		return false, err
	}
	defer file.Close()
	_, err = io.Copy(file, body)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (b *LocalBucket) GetUpdate(branch string, runtimeVersion string, updateId string) (*types.Update, error) {
	if b.BasePath == "" {
		return nil, errors.New("BasePath not set")
	}

	metadataPath := filepath.Join(b.BasePath, branch, runtimeVersion, updateId, "metadata.json")
	file, err := os.Open(metadataPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var metadata types.UpdateMetadata
	if err := json.NewDecoder(file).Decode(&metadata); err != nil {
		return nil, err
	}

	createdAt, err := time.ParseDuration(metadata.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CreatedAt: %w", err)
	}

	return &types.Update{
		Branch:         branch,
		RuntimeVersion: runtimeVersion,
		UpdateId:       updateId,
		CreatedAt:      createdAt,
	}, nil
}

func (b *LocalBucket) RequestUploadUrlsForFileUpdates(branch string, runtimeVersion string, updateId string, fileNames []string) ([]types.FileUpdateRequest, error) {
	if b.BasePath == "" {
		return nil, errors.New("BasePath not set")
	}

	var requests []types.FileUpdateRequest
	for _, fileName := range fileNames {
		url, err := b.RequestUploadUrlForFileUpdate(branch, runtimeVersion, updateId, fileName)
		if err != nil {
			return nil, fmt.Errorf("error generating upload URL for %s: %w", fileName, err)
		}

		filePath := filepath.Join(branch, runtimeVersion, updateId, fileName)
		requests = append(requests, types.FileUpdateRequest{
			Url:  url,
			Path: filePath,
		})
	}

	return requests, nil
}

// ListUpdates returns a list of all update IDs for a specific branch and runtime version
func (b *LocalBucket) ListUpdates(branch string, runtimeVersion string) ([]string, error) {
	if b.BasePath == "" {
		return nil, errors.New("BasePath not set")
	}

	dirPath := filepath.Join(b.BasePath, branch, runtimeVersion)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return []string{}, nil
	}

	var updates []string
	for _, entry := range entries {
		if entry.IsDir() {
			updates = append(updates, entry.Name())
		}
	}

	return updates, nil
}

func (lb *LocalBucket) DeleteFile(branch string, runtimeVersion string, updateId string, fileName string) error {
	// Construct the full path to the file
	filePath := fmt.Sprintf("%s/%s/%s/%s/%s", lb.BasePath, branch, runtimeVersion, updateId, fileName)

	// Check if file exists and remove it
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// File doesn't exist, not an error for deletion
		return nil
	}

	// Delete the file
	return os.Remove(filePath)
}

func (lb *LocalBucket) StoreUpdateDownload(download types.UpdateDownload) error {
	// Create downloads directory if it doesn't exist
	downloadsDir := fmt.Sprintf("%s/downloads/%s/%s/%s",
		lb.BasePath, download.Branch, download.RuntimeVersion, download.UpdateId)
	os.MkdirAll(downloadsDir, 0755)

	// Create a download record with timestamp in filename
	downloadFileName := fmt.Sprintf("%s/%s_%s.json",
		downloadsDir, download.UserId, download.DownloadedAt)

	// Convert download record to JSON
	downloadData, err := json.Marshal(download)
	if err != nil {
		return fmt.Errorf("error marshaling download record: %w", err)
	}

	// Write to file
	err = os.WriteFile(downloadFileName, downloadData, 0644)
	if err != nil {
		return fmt.Errorf("error writing download record: %w", err)
	}

	return nil
}

func (lb *LocalBucket) GetUpdateDownloads(branch string, runtimeVersion string, updateId string) ([]types.UpdateDownload, error) {
	// Path to downloads directory
	downloadsDir := fmt.Sprintf("%s/downloads/%s/%s/%s",
		lb.BasePath, branch, runtimeVersion, updateId)

	// Check if directory exists
	if _, err := os.Stat(downloadsDir); os.IsNotExist(err) {
		// No downloads yet, return empty list
		return []types.UpdateDownload{}, nil
	}

	// Read all download files
	files, err := os.ReadDir(downloadsDir)
	if err != nil {
		return nil, fmt.Errorf("error reading downloads directory: %w", err)
	}

	downloads := make([]types.UpdateDownload, 0, len(files))

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		// Read download record
		downloadData, err := os.ReadFile(fmt.Sprintf("%s/%s", downloadsDir, file.Name()))
		if err != nil {
			log.Printf("Error reading download record %s: %v", file.Name(), err)
			continue
		}

		// Parse JSON
		var download types.UpdateDownload
		if err := json.Unmarshal(downloadData, &download); err != nil {
			log.Printf("Error parsing download record %s: %v", file.Name(), err)
			continue
		}

		downloads = append(downloads, download)
	}

	return downloads, nil
}

func (lb *LocalBucket) ActivateUpdate(branch string, runtimeVersion string, updateId string) error {
	// Get update to ensure it exists
	update, err := lb.GetUpdate(branch, runtimeVersion, updateId)
	if err != nil {
		return fmt.Errorf("error getting update: %w", err)
	}

	// Set active flag
	update.Active = true

	// Create multiple active marker files for compatibility with different client implementations

	// Root update directory
	rootActivePath := fmt.Sprintf("%s/%s/%s/%s/.active",
		lb.BasePath, branch, runtimeVersion, updateId)
	if err := os.WriteFile(rootActivePath, []byte("active"), 0644); err != nil {
		log.Printf("Warning: Error creating root active marker: %v", err)
	}

	// Also store without dot prefix
	rootActivePathNoDot := fmt.Sprintf("%s/%s/%s/%s/active",
		lb.BasePath, branch, runtimeVersion, updateId)
	if err := os.WriteFile(rootActivePathNoDot, []byte("active"), 0644); err != nil {
		log.Printf("Warning: Error creating root active marker (no dot): %v", err)
	}

	// Assets directory
	assetsDir := fmt.Sprintf("%s/%s/%s/%s/assets",
		lb.BasePath, branch, runtimeVersion, updateId)

	// Create assets directory if it doesn't exist
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		log.Printf("Warning: Error creating assets directory: %v", err)
	} else {
		// Assets directory active marker
		assetsActivePath := fmt.Sprintf("%s/active", assetsDir)
		if err := os.WriteFile(assetsActivePath, []byte("active"), 0644); err != nil {
			log.Printf("Warning: Error creating assets active marker: %v", err)
		}
	}

	// Remove all inactive markers
	inactivePaths := []string{
		fmt.Sprintf("%s/%s/%s/%s/.inactive", lb.BasePath, branch, runtimeVersion, updateId),
		fmt.Sprintf("%s/%s/%s/%s/inactive", lb.BasePath, branch, runtimeVersion, updateId),
		fmt.Sprintf("%s/%s/%s/%s/assets/inactive", lb.BasePath, branch, runtimeVersion, updateId),
	}

	for _, path := range inactivePaths {
		os.Remove(path)
	}

	return nil
}

func (lb *LocalBucket) DeactivateUpdate(branch string, runtimeVersion string, updateId string) error {
	// Get update to ensure it exists
	update, err := lb.GetUpdate(branch, runtimeVersion, updateId)
	if err != nil {
		return fmt.Errorf("error getting update: %w", err)
	}

	// Set active flag
	update.Active = false

	// Create multiple inactive marker files for compatibility with different client implementations

	// Root update directory
	rootInactivePath := fmt.Sprintf("%s/%s/%s/%s/.inactive",
		lb.BasePath, branch, runtimeVersion, updateId)
	if err := os.WriteFile(rootInactivePath, []byte("inactive"), 0644); err != nil {
		log.Printf("Warning: Error creating root inactive marker: %v", err)
	}

	// Also store without dot prefix
	rootInactivePathNoDot := fmt.Sprintf("%s/%s/%s/%s/inactive",
		lb.BasePath, branch, runtimeVersion, updateId)
	if err := os.WriteFile(rootInactivePathNoDot, []byte("inactive"), 0644); err != nil {
		log.Printf("Warning: Error creating root inactive marker (no dot): %v", err)
	}

	// Assets directory
	assetsDir := fmt.Sprintf("%s/%s/%s/%s/assets",
		lb.BasePath, branch, runtimeVersion, updateId)

	// Create assets directory if it doesn't exist
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		log.Printf("Warning: Error creating assets directory: %v", err)
	} else {
		// Assets directory inactive marker
		assetsInactivePath := fmt.Sprintf("%s/inactive", assetsDir)
		if err := os.WriteFile(assetsInactivePath, []byte("inactive"), 0644); err != nil {
			log.Printf("Warning: Error creating assets inactive marker: %v", err)
		}
	}

	// Remove all active markers
	activePaths := []string{
		fmt.Sprintf("%s/%s/%s/%s/.active", lb.BasePath, branch, runtimeVersion, updateId),
		fmt.Sprintf("%s/%s/%s/%s/active", lb.BasePath, branch, runtimeVersion, updateId),
		fmt.Sprintf("%s/%s/%s/%s/assets/active", lb.BasePath, branch, runtimeVersion, updateId),
	}

	for _, path := range activePaths {
		os.Remove(path)
	}

	return nil
}

func (lb *LocalBucket) GetActiveUpdates(branch string, runtimeVersion string) ([]types.Update, error) {
	// Get all updates for the branch and version
	updates, err := lb.GetUpdates(branch, runtimeVersion)
	if err != nil {
		return nil, fmt.Errorf("error getting updates: %w", err)
	}

	// Filter for active updates
	activeUpdates := make([]types.Update, 0, len(updates))

	for _, update := range updates {
		// Check for active marker or absence of inactive marker
		activeFilePath := fmt.Sprintf("%s/%s/%s/%s/.active",
			lb.BasePath, branch, runtimeVersion, update.UpdateId)
		inactiveFilePath := fmt.Sprintf("%s/%s/%s/%s/.inactive",
			lb.BasePath, branch, runtimeVersion, update.UpdateId)

		if _, err := os.Stat(activeFilePath); err == nil {
			// Active marker exists
			update.Active = true
			activeUpdates = append(activeUpdates, update)
		} else if _, err := os.Stat(inactiveFilePath); os.IsNotExist(err) {
			// Inactive marker doesn't exist, consider active by default
			update.Active = true
			activeUpdates = append(activeUpdates, update)
		} else {
			// Inactive marker exists
			update.Active = false
		}
	}

	return activeUpdates, nil
}
