package bucket

import (
	"bytes"
	"expo-open-ota/config"
	"expo-open-ota/internal/types"
	"fmt"
	"io"
	"log"
	"os"
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
var bucketInitError error

func init() {
	// Get bucket configuration
	bucketType := config.GetEnv("BUCKET_TYPE")
	storageMode := config.GetEnv("STORAGE_MODE")

	// Align storage mode with bucket type if needed
	if bucketType != "" && (storageMode == "" || storageMode != string(bucketType)) {
		log.Printf("STORAGE_MODE (%s) doesn't match BUCKET_TYPE (%s), setting STORAGE_MODE=%s",
			storageMode, bucketType, bucketType)
		// Update the storage mode to match bucket type
		os.Setenv("STORAGE_MODE", string(bucketType))
		storageMode = string(bucketType)
	} else if bucketType == "" && storageMode != "" {
		// If bucket type is not set but storage mode is, set bucket type to match storage mode
		log.Printf("BUCKET_TYPE not set but STORAGE_MODE is %s, setting BUCKET_TYPE=%s", storageMode, storageMode)
		os.Setenv("BUCKET_TYPE", storageMode)
		bucketType = storageMode
	}

	if bucketType == "" {
		bucketType = string(LocalBucketType)
		log.Printf("No BUCKET_TYPE or STORAGE_MODE specified, using default: %s", bucketType)
		os.Setenv("BUCKET_TYPE", bucketType)
		os.Setenv("STORAGE_MODE", bucketType)
	} else {
		log.Printf("Using bucket type: %s", bucketType)
	}

	// First try to initialize the configured bucket type
	var initErr error
	switch BucketType(bucketType) {
	case LocalBucketType:
		log.Printf("Initializing local bucket with path: %s", config.GetEnv("LOCAL_BUCKET_BASE_PATH"))
		bucket = NewLocalBucket()
	case FirebaseBucketType:
		log.Printf("Initializing Firebase bucket (storage bucket: %s, project ID: %s)",
			config.GetEnv("FIREBASE_STORAGE_BUCKET"),
			config.GetEnv("FIREBASE_PROJECT_ID"))
		var err error
		bucket, err = NewFirebaseBucket()
		if err != nil {
			initErr = fmt.Errorf("error creating Firebase bucket: %w", err)
			log.Printf("Firebase initialization error details: %v", err)

			// Check for common configuration issues
			if config.GetEnv("FIREBASE_PROJECT_ID") == "" && config.GetEnv("FIREBASE_SERVICE_ACCOUNT") == "" {
				log.Printf("ERROR: Neither FIREBASE_PROJECT_ID nor FIREBASE_SERVICE_ACCOUNT is set")
			}
			if config.GetEnv("FIREBASE_STORAGE_BUCKET") == "" {
				log.Printf("WARNING: FIREBASE_STORAGE_BUCKET is not set")
			}
		}
	case S3BucketType:
		log.Printf("Initializing S3 bucket: %s in region %s", config.GetEnv("S3_BUCKET_NAME"), config.GetEnv("AWS_REGION"))
		bucket = &S3Bucket{BucketName: config.GetEnv("S3_BUCKET_NAME")}
	default:
		initErr = fmt.Errorf("unknown bucket type: %s", bucketType)
	}

	// If initialization failed, fall back to a local bucket
	if initErr != nil {
		log.Printf("Error initializing bucket of type %s: %v", bucketType, initErr)
		log.Printf("Falling back to a local bucket")
		bucket = NewLocalBucket()
		bucketInitError = initErr
	} else {
		log.Printf("Successfully initialized bucket of type: %s", bucketType)
	}
}

func GetBucket() Bucket {
	if bucketInitError != nil {
		log.Printf("WARNING: Using fallback bucket due to initialization error: %v", bucketInitError)
	}
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
