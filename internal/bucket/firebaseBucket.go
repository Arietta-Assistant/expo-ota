package bucket

import (
	"context"
	"expo-open-ota/config"
	"expo-open-ota/internal/types"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

type FirebaseBucket struct {
	client *storage.Client
	bucket *storage.BucketHandle
}

func NewFirebaseBucket() (*FirebaseBucket, error) {
	ctx := context.Background()

	// Get Firebase credentials from environment variables
	credentials := []byte(config.GetEnv("FIREBASE_CREDENTIALS"))
	if len(credentials) == 0 {
		return nil, fmt.Errorf("FIREBASE_CREDENTIALS environment variable is not set")
	}

	client, err := storage.NewClient(ctx, option.WithCredentialsJSON(credentials))
	if err != nil {
		return nil, fmt.Errorf("error creating Firebase Storage client: %w", err)
	}

	bucketName := config.GetEnv("FIREBASE_STORAGE_BUCKET")
	if bucketName == "" {
		return nil, fmt.Errorf("FIREBASE_STORAGE_BUCKET environment variable is not set")
	}

	bucket := client.Bucket(bucketName)

	return &FirebaseBucket{
		client: client,
		bucket: bucket,
	}, nil
}

func (b *FirebaseBucket) GetUpdate(branch string, runtimeVersion string, updateId string) (*types.Update, error) {
	objectPath := path.Join("updates", branch, runtimeVersion, updateId, "update-metadata.json")
	reader, err := b.bucket.Object(objectPath).NewReader(context.Background())
	if err != nil {
		return nil, fmt.Errorf("error reading update metadata: %w", err)
	}
	defer reader.Close()

	// Read and parse the metadata
	// ... implementation similar to localBucket.go ...
	return nil, nil
}

func (b *FirebaseBucket) GetFile(branch string, runtimeVersion string, updateId string, fileName string) (io.ReadCloser, error) {
	objectPath := path.Join("updates", branch, runtimeVersion, updateId, fileName)
	return b.bucket.Object(objectPath).NewReader(context.Background())
}

func (b *FirebaseBucket) UploadFileIntoUpdate(update types.Update, fileName string, content io.Reader) error {
	objectPath := path.Join("updates", update.Branch, update.RuntimeVersion, update.UpdateId, fileName)
	writer := b.bucket.Object(objectPath).NewWriter(context.Background())
	defer writer.Close()

	_, err := io.Copy(writer, content)
	return err
}

func (b *FirebaseBucket) DeleteUpdateFolder(branch string, runtimeVersion string, updateId string) error {
	prefix := path.Join("updates", branch, runtimeVersion, updateId)
	query := &storage.Query{
		Prefix: prefix,
	}
	iter := b.bucket.Objects(context.Background(), query)

	for {
		attrs, err := iter.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error iterating objects: %w", err)
		}

		if err := b.bucket.Object(attrs.Name).Delete(context.Background()); err != nil {
			return fmt.Errorf("error deleting object %s: %w", attrs.Name, err)
		}
	}

	return nil
}

func (b *FirebaseBucket) RequestUploadUrlsForFileUpdates(branch string, runtimeVersion string, updateId string, fileNames []string) ([]types.FileUpdateRequest, error) {
	var requests []types.FileUpdateRequest

	for _, fileName := range fileNames {
		objectPath := path.Join("updates", branch, runtimeVersion, updateId, fileName)
		opts := &storage.SignedURLOptions{
			Method:  "PUT",
			Headers: []string{"Content-Type: application/octet-stream"},
		}
		url, err := storage.SignedURL("your-firebase-storage-bucket-name", objectPath, opts)
		if err != nil {
			return nil, fmt.Errorf("error generating signed URL for %s: %w", fileName, err)
		}

		requests = append(requests, types.FileUpdateRequest{
			Url:  url,
			Path: objectPath,
		})
	}

	return requests, nil
}

func (b *FirebaseBucket) GetUpdates(branch string, runtimeVersion string) ([]types.Update, error) {
	prefix := path.Join("updates", branch, runtimeVersion)
	query := &storage.Query{
		Prefix: prefix,
	}
	iter := b.bucket.Objects(context.Background(), query)

	var updates []types.Update
	for {
		attrs, err := iter.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error iterating objects: %w", err)
		}

		// Extract updateId from the path
		parts := strings.Split(attrs.Name, "/")
		if len(parts) >= 4 {
			updateId := parts[3]
			update := types.Update{
				Branch:         branch,
				RuntimeVersion: runtimeVersion,
				UpdateId:       updateId,
				CreatedAt:      time.Duration(attrs.Created.UnixNano()),
			}
			updates = append(updates, update)
		}
	}

	return updates, nil
}

func (b *FirebaseBucket) GetBranches() ([]string, error) {
	prefix := "updates/"
	query := &storage.Query{
		Prefix:    prefix,
		Delimiter: "/",
	}
	iter := b.bucket.Objects(context.Background(), query)

	var branches []string
	for {
		attrs, err := iter.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error iterating objects: %w", err)
		}

		if attrs.Prefix != "" {
			// Extract branch name from prefix (e.g., "updates/main/" -> "main")
			branch := strings.TrimPrefix(attrs.Prefix, prefix)
			branch = strings.TrimSuffix(branch, "/")
			if branch != "" {
				branches = append(branches, branch)
			}
		}
	}

	return branches, nil
}

func (b *FirebaseBucket) GetRuntimeVersions(branch string) ([]RuntimeVersionWithStats, error) {
	prefix := path.Join("updates", branch) + "/"
	query := &storage.Query{
		Prefix:    prefix,
		Delimiter: "/",
	}
	iter := b.bucket.Objects(context.Background(), query)

	runtimeVersions := make(map[string]*RuntimeVersionWithStats)
	for {
		attrs, err := iter.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error iterating objects: %w", err)
		}

		if attrs.Prefix != "" {
			// Extract runtime version from prefix (e.g., "updates/main/1.0.0/" -> "1.0.0")
			version := strings.TrimPrefix(attrs.Prefix, prefix)
			version = strings.TrimSuffix(version, "/")
			if version != "" {
				// Get updates for this runtime version to count them
				updates, err := b.GetUpdates(branch, version)
				if err != nil {
					return nil, err
				}

				var lastUpdatedAt time.Time
				for _, update := range updates {
					updateTime := time.Unix(0, int64(update.CreatedAt))
					if updateTime.After(lastUpdatedAt) {
						lastUpdatedAt = updateTime
					}
				}

				runtimeVersions[version] = &RuntimeVersionWithStats{
					RuntimeVersion:  version,
					LastUpdatedAt:   lastUpdatedAt.UTC().Format(time.RFC3339),
					CreatedAt:       lastUpdatedAt.UTC().Format(time.RFC3339), // Using lastUpdatedAt as createdAt since we don't have the actual creation time
					NumberOfUpdates: len(updates),
				}
			}
		}
	}

	// Convert map to slice
	var result []RuntimeVersionWithStats
	for _, stats := range runtimeVersions {
		result = append(result, *stats)
	}

	return result, nil
}
