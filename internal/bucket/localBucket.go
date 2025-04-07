package bucket

import (
	"encoding/json"
	"errors"
	"expo-open-ota/config"
	"expo-open-ota/internal/services"
	"expo-open-ota/internal/types"
	"fmt"
	"io"
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
	return branches, nil
}

func (b *LocalBucket) GetRuntimeVersions(branch string) ([]RuntimeVersionWithStats, error) {
	if b.BasePath == "" {
		return nil, errors.New("BasePath not set")
	}
	dirPath := filepath.Join(b.BasePath, branch)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	var runtimeVersions []RuntimeVersionWithStats
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runtimeVersion := entry.Name()
		updatesPath := filepath.Join(dirPath, runtimeVersion)
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

	return runtimeVersions, nil
}

func (b *LocalBucket) UploadFileIntoUpdate(update types.Update, fileName string, file io.Reader) error {
	filePath := filepath.Join(b.BasePath, update.Branch, update.RuntimeVersion, update.UpdateId, fileName)
	err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
	if err != nil {
		return err
	}
	out, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, file)
	if err != nil {
		return err
	}
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
