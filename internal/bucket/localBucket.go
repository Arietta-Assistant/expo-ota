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
