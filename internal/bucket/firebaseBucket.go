package bucket

import (
	"context"
	"expo-open-ota/internal/types"
	"fmt"
	"io"
	"path"
	"time"

	"firebase.google.com/go/storage"
)

type FirebaseBucket struct {
	client *storage.Client
	bucket *storage.BucketHandle
}

func NewFirebaseBucket() (*FirebaseBucket, error) {
	client, err := storage.NewClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("error creating Firebase Storage client: %w", err)
	}

	bucket := client.Bucket("your-firebase-storage-bucket-name") // Replace with your bucket name

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
	iter := b.bucket.Objects(context.Background(), &storage.Query{
		Prefix: prefix,
	})

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
	now := time.Now().UnixNano() / int64(time.Millisecond)

	for _, fileName := range fileNames {
		objectPath := path.Join("updates", branch, runtimeVersion, updateId, fileName)
		url, err := b.bucket.Object(objectPath).SignedURL(context.Background(), now+3600000, &storage.SignedURLOptions{
			Method:  "PUT",
			Headers: []string{"Content-Type: application/octet-stream"},
		})
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
