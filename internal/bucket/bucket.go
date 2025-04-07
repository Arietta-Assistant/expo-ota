package bucket

import (
	"bytes"
	"expo-open-ota/config"
	"expo-open-ota/internal/types"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"sync"
)

type RuntimeVersionWithStats struct {
	RuntimeVersion  string `json:"runtimeVersion"`
	LastUpdatedAt   string `json:"lastUpdatedAt"`
	CreatedAt       string `json:"createdAt"`
	NumberOfUpdates int    `json:"numberOfUpdates"`
}

type BucketType string

const (
	S3BucketType       BucketType = "s3"
	LocalBucketType    BucketType = "local"
	FirebaseBucketType BucketType = "firebase"
)

type Bucket interface {
	GetUpdate(branch string, runtimeVersion string, updateId string) (*types.Update, error)
	GetFile(branch string, runtimeVersion string, updateId string, fileName string) (io.ReadCloser, error)
	UploadFileIntoUpdate(update types.Update, fileName string, content io.Reader) error
	DeleteUpdateFolder(branch string, runtimeVersion string, updateId string) error
	RequestUploadUrlsForFileUpdates(branch string, runtimeVersion string, updateId string, fileNames []string) ([]types.FileUpdateRequest, error)
}

var bucket Bucket

func init() {
	bucketType := config.GetEnv("BUCKET_TYPE")
	if bucketType == "" {
		bucketType = string(FirebaseBucketType)
	}

	var err error
	switch BucketType(bucketType) {
	case FirebaseBucketType:
		bucket, err = NewFirebaseBucket()
		if err != nil {
			log.Fatalf("Error creating Firebase bucket: %v", err)
		}
	default:
		log.Fatalf("Unknown bucket type: %s", bucketType)
	}
}

func GetBucket() Bucket {
	return bucket
}

func ConvertReadCloserToBytes(rc io.ReadCloser) ([]byte, error) {
	defer rc.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rc); err != nil {
		return nil, fmt.Errorf("error copying file to buffer: %w", err)
	}
	return buf.Bytes(), nil
}

func ResetBucketInstance() {
	bucket = nil
}

type FileUploadRequest struct {
	RequestUploadUrl string `json:"requestUploadUrl"`
	FileName         string `json:"fileName"`
	FilePath         string `json:"filePath"`
}

func RequestUploadUrlsForFileUpdates(branch string, runtimeVersion string, updateId string, fileNames []string) ([]FileUploadRequest, error) {
	uniqueFileNames := make(map[string]struct{})
	for _, fileName := range fileNames {
		uniqueFileNames[fileName] = struct{}{}
	}

	bucket := GetBucket()

	var requests []FileUploadRequest
	var mu sync.Mutex
	var wg sync.WaitGroup
	errChan := make(chan error, len(uniqueFileNames))

	wg.Add(len(uniqueFileNames))
	for fileName := range uniqueFileNames {
		go func(fileName string) {
			defer wg.Done()
			requestUploadUrl, err := bucket.RequestUploadUrlForFileUpdate(branch, runtimeVersion, updateId, fileName)
			if err != nil {
				errChan <- err
				return
			}
			mu.Lock()
			requests = append(requests, FileUploadRequest{
				RequestUploadUrl: requestUploadUrl,
				FileName:         filepath.Base(fileName),
				FilePath:         fileName,
			})
			mu.Unlock()
		}(fileName)
	}

	wg.Wait()
	close(errChan)

	if len(errChan) > 0 {
		return nil, <-errChan
	}

	return requests, nil
}
