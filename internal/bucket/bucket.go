package bucket

import (
	"bytes"
	"expo-open-ota/config"
	"expo-open-ota/internal/types"
	"fmt"
	"io"
	"log"
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
	GetUpdates(branch string, runtimeVersion string) ([]types.Update, error)
	GetBranches() ([]string, error)
	GetRuntimeVersions(branch string) ([]RuntimeVersionWithStats, error)
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
			fileRequests, err := bucket.RequestUploadUrlsForFileUpdates(branch, runtimeVersion, updateId, []string{fileName})
			if err != nil {
				errChan <- err
				return
			}
			if len(fileRequests) > 0 {
				mu.Lock()
				requests = append(requests, FileUploadRequest{
					RequestUploadUrl: fileRequests[0].Url,
					FileName:         fileName,
					FilePath:         fileRequests[0].Path,
				})
				mu.Unlock()
			}
		}(fileName)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			return nil, err
		}
	}

	return requests, nil
}
