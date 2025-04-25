package bucket

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"expo-open-ota/config"
	"expo-open-ota/internal/types"
	"fmt"
	"io"
	"log"
	"path"
	"strings"
	"time"

	"bytes"

	"cloud.google.com/go/storage"
	firebase "firebase.google.com/go/v4"
	firebaseStorage "firebase.google.com/go/v4/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type FirebaseBucket struct {
	firebaseClient *firebaseStorage.Client
	bucket         *storage.BucketHandle
}

func NewFirebaseBucket() (*FirebaseBucket, error) {
	ctx := context.Background()

	// Get Firebase credentials from environment variables
	base64Credentials := config.GetEnv("FIREBASE_SERVICE_ACCOUNT")
	if base64Credentials == "" {
		log.Printf("Warning: FIREBASE_SERVICE_ACCOUNT environment variable is not set")
		// Try alternative approaches
		projectID := config.GetEnv("FIREBASE_PROJECT_ID")
		if projectID == "" {
			return nil, fmt.Errorf("missing required Firebase configuration (either FIREBASE_SERVICE_ACCOUNT or FIREBASE_PROJECT_ID)")
		}

		// If we have project ID but no credentials, try to use default credentials
		log.Printf("Attempting to initialize Firebase with project ID but no explicit credentials")

		// Initialize with project ID
		app, err := firebase.NewApp(ctx, &firebase.Config{
			ProjectID: projectID,
		})
		if err != nil {
			return nil, fmt.Errorf("error initializing Firebase app: %w", err)
		}

		// Get the storage client
		firebaseClient, err := app.Storage(ctx)
		if err != nil {
			return nil, fmt.Errorf("error initializing Firebase storage client: %w", err)
		}

		// Get bucket name
		bucketName := config.GetEnv("FIREBASE_STORAGE_BUCKET")
		if bucketName == "" {
			bucketName = projectID + ".appspot.com"
			log.Printf("FIREBASE_STORAGE_BUCKET not set, using default: %s", bucketName)
		}

		bucket, err := firebaseClient.Bucket(bucketName)
		if err != nil {
			return nil, fmt.Errorf("error accessing Firebase storage bucket: %w", err)
		}

		log.Printf("Successfully initialized Firebase bucket using project ID: %s", projectID)
		return &FirebaseBucket{
			firebaseClient: firebaseClient,
			bucket:         bucket,
		}, nil
	}

	// Decode base64 credentials if provided
	credentials, err := base64.StdEncoding.DecodeString(base64Credentials)
	if err != nil {
		return nil, fmt.Errorf("error decoding Firebase credentials: %w", err)
	}

	// Initialize with credentials
	app, err := firebase.NewApp(ctx, nil, option.WithCredentialsJSON(credentials))
	if err != nil {
		return nil, fmt.Errorf("error initializing Firebase app with credentials: %w", err)
	}

	// Get storage client
	firebaseClient, err := app.Storage(ctx)
	if err != nil {
		return nil, fmt.Errorf("error initializing Firebase storage client: %w", err)
	}

	// Get bucket name
	bucketName := config.GetEnv("FIREBASE_STORAGE_BUCKET")
	if bucketName == "" {
		// Try to get project ID from the credentials to derive bucket name
		var creds map[string]interface{}
		if err := json.Unmarshal(credentials, &creds); err == nil {
			if projectID, ok := creds["project_id"].(string); ok && projectID != "" {
				bucketName = projectID + ".appspot.com"
				log.Printf("Derived bucket name from credentials: %s", bucketName)
			}
		}

		if bucketName == "" {
			return nil, fmt.Errorf("FIREBASE_STORAGE_BUCKET environment variable is not set and could not be derived")
		}
	}

	bucket, err := firebaseClient.Bucket(bucketName)
	if err != nil {
		return nil, fmt.Errorf("error accessing Firebase storage bucket: %w", err)
	}

	log.Printf("Successfully initialized Firebase bucket using service account credentials")
	return &FirebaseBucket{
		firebaseClient: firebaseClient,
		bucket:         bucket,
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
	log.Printf("DEBUG: Getting file %s", objectPath)

	// Special debug for metadata.json
	if fileName == "metadata.json" {
		reader, err := b.bucket.Object(objectPath).NewReader(context.Background())
		if err != nil {
			log.Printf("DEBUG: Error reading metadata file: %v", err)
			return nil, fmt.Errorf("error reading metadata file: %w", err)
		}

		// Read the content
		content, err := io.ReadAll(reader)
		if err != nil {
			reader.Close()
			log.Printf("DEBUG: Error reading metadata content: %v", err)
			return nil, fmt.Errorf("error reading metadata content: %w", err)
		}

		// Log the content
		log.Printf("DEBUG: Metadata file content for %s/%s/%s: %s",
			branch, runtimeVersion, updateId, string(content))

		// Reset the reader
		reader.Close()

		// Create a new reader with the same content
		return io.NopCloser(bytes.NewReader(content)), nil
	}

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

		// Determine content type based on file extension
		contentType := "application/octet-stream"
		if strings.HasSuffix(fileName, ".json") {
			contentType = "application/json"
		} else if strings.HasSuffix(fileName, ".js") {
			contentType = "application/javascript"
		} else if strings.HasSuffix(fileName, ".png") {
			contentType = "image/png"
		} else if strings.HasSuffix(fileName, ".jpg") || strings.HasSuffix(fileName, ".jpeg") {
			contentType = "image/jpeg"
		}

		// Create signed URL with V4 signing scheme
		opts := &storage.SignedURLOptions{
			Method:      "PUT",
			ContentType: contentType, // Set content type directly
			Expires:     time.Now().Add(15 * time.Minute),
			Scheme:      storage.SigningSchemeV4, // Explicitly use V4 signing
		}

		// Generate the signed URL
		url, err := b.bucket.SignedURL(objectPath, opts)
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
	var result []types.Update
	objectPath := path.Join("updates", branch, runtimeVersion)
	log.Printf("Querying for updates in path: %s", objectPath)

	// List all objects in the specified path
	iter := b.bucket.Objects(context.Background(), &storage.Query{
		Prefix:    objectPath,
		Delimiter: "/",
	})

	var paths []string
	// First pass: collect all update IDs
	for {
		attrs, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Error listing objects: %v", err)
			return nil, err
		}

		// For directory prefixes, extract the update ID
		if attrs.Prefix != "" {
			updateID := path.Base(strings.TrimSuffix(attrs.Prefix, "/"))
			paths = append(paths, updateID)
			log.Printf("Found update ID: %s", updateID)
		}
	}

	log.Printf("Found %d potential update IDs in %s", len(paths), objectPath)

	// Special handling for a specific update ID format
	for _, updateID := range paths {
		if strings.HasPrefix(updateID, "build-7-") {
			log.Printf("IMPORTANT: Found update with build-7 prefix: %s", updateID)
		}
	}

	// Second pass: process each update ID
	for _, updateID := range paths {
		// Try to read metadata for this update ID
		metadataPath := path.Join(objectPath, updateID, "metadata.json")

		log.Printf("Looking for metadata at: %s", metadataPath)
		reader, err := b.bucket.Object(metadataPath).NewReader(context.Background())
		if err != nil {
			log.Printf("Error reading metadata for update %s: %v", updateID, err)
			continue
		}
		defer reader.Close()

		// Try to parse the metadata
		var metadata map[string]interface{}
		metadataContent, err := io.ReadAll(reader)
		if err != nil {
			log.Printf("Error reading metadata content for update %s: %v", updateID, err)
			continue
		}

		log.Printf("Successfully read metadata file for update: %s", updateID)

		if err := json.Unmarshal(metadataContent, &metadata); err != nil {
			log.Printf("Error parsing metadata JSON for update %s: %v", updateID, err)
			continue
		}

		// Check for "extra" map to extract buildNumber
		var buildNumber string
		if extra, ok := metadata["extra"].(map[string]interface{}); ok {
			if bn, ok := extra["buildNumber"]; ok {
				buildNumber = fmt.Sprintf("%v", bn)
				log.Printf("Found buildNumber %s for update %s", buildNumber, updateID)
			} else {
				log.Printf("No buildNumber found in extra for update %s", updateID)
			}
		} else if metadataObj, ok := metadata["metadata"].(map[string]interface{}); ok {
			if extra, ok := metadataObj["extra"].(map[string]interface{}); ok {
				if bn, ok := extra["buildNumber"]; ok {
					buildNumber = fmt.Sprintf("%v", bn)
					log.Printf("Found buildNumber %s in nested metadata for update %s", buildNumber, updateID)
				} else {
					log.Printf("No buildNumber found in nested metadata.extra for update %s", updateID)
				}
			}
		}

		// Create the update object
		update := types.Update{
			Branch:         branch,
			RuntimeVersion: runtimeVersion,
			UpdateId:       updateID,
		}

		// Parse creation time from update ID if possible
		if strings.HasPrefix(updateID, "build-") {
			parts := strings.Split(updateID, "-")
			if len(parts) > 1 {
				buildNumberStr := parts[1]
				log.Printf("Extracted build number part from ID: %s", buildNumberStr)
			}
		}

		// Always add the update to results regardless of buildNumber
		log.Printf("Adding update %s to results", updateID)
		result = append(result, update)
	}

	log.Printf("Completed GetUpdates, found %d updates for %s/%s", len(result), branch, runtimeVersion)
	for i, update := range result {
		log.Printf("Result update #%d: %s", i+1, update.UpdateId)
	}

	return result, nil
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

// DumpUpdateMetadata dumps the complete metadata for a specific update ID
func (b *FirebaseBucket) DumpUpdateMetadata(branch string, runtimeVersion string, updateId string) {
	objectPath := path.Join("updates", branch, runtimeVersion, updateId, "metadata.json")
	log.Printf("DUMP: Attempting to read metadata for update %s at path %s", updateId, objectPath)

	reader, err := b.bucket.Object(objectPath).NewReader(context.Background())
	if err != nil {
		log.Printf("DUMP: Error opening metadata file: %v", err)
		return
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		log.Printf("DUMP: Error reading metadata content: %v", err)
		return
	}

	// Pretty print the JSON
	var prettyJSON bytes.Buffer
	err = json.Indent(&prettyJSON, content, "", "  ")
	if err != nil {
		log.Printf("DUMP: Error formatting JSON: %v", err)
		log.Printf("DUMP: Raw metadata content: %s", string(content))
		return
	}

	log.Printf("DUMP: Complete metadata for update %s:\n%s", updateId, prettyJSON.String())

	// Also try to parse and extract specific fields
	var metadata map[string]interface{}
	if err := json.Unmarshal(content, &metadata); err == nil {
		log.Printf("DUMP: Metadata root keys: %v", getMapKeys(metadata))

		// Try to access extra directly
		if extra, ok := metadata["extra"].(map[string]interface{}); ok {
			log.Printf("DUMP: extra fields: %v", getMapKeys(extra))
			log.Printf("DUMP: extra.buildNumber = %v", extra["buildNumber"])
			log.Printf("DUMP: extra.updateCode = %v", extra["updateCode"])
		}

		// Try to access nested metadata
		if metadataObj, ok := metadata["metadata"].(map[string]interface{}); ok {
			log.Printf("DUMP: metadata.metadata fields: %v", getMapKeys(metadataObj))

			if extra, ok := metadataObj["extra"].(map[string]interface{}); ok {
				log.Printf("DUMP: metadata.metadata.extra fields: %v", getMapKeys(extra))
				log.Printf("DUMP: metadata.metadata.extra.buildNumber = %v", extra["buildNumber"])
				log.Printf("DUMP: metadata.metadata.extra.updateCode = %v", extra["updateCode"])
			}
		}
	} else {
		log.Printf("DUMP: Error parsing JSON: %v", err)
	}
}

// Helper function to get keys from a map
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// GetDirectMetadata gets the metadata for a specific update ID by directly constructing possible paths
func (b *FirebaseBucket) GetDirectMetadata(updateId string) {
	log.Printf("Attempting direct metadata access for update ID: %s", updateId)

	// Log Firebase credential status
	bucketName := config.GetEnv("FIREBASE_STORAGE_BUCKET")
	log.Printf("Using Firebase bucket: %s", bucketName)

	// Check if credentials are set
	base64Credentials := config.GetEnv("FIREBASE_SERVICE_ACCOUNT")
	if base64Credentials == "" {
		log.Printf("WARNING: FIREBASE_SERVICE_ACCOUNT environment variable is not set")
	} else {
		log.Printf("FIREBASE_SERVICE_ACCOUNT is set (length: %d)", len(base64Credentials))
	}

	// First, list all objects to find any match for this update ID
	ctx := context.Background()
	debugQuery := &storage.Query{
		Prefix: "updates/",
	}

	log.Printf("Listing all objects in bucket to find update ID: %s", updateId)
	debugIter := b.bucket.Objects(ctx, debugQuery)

	var matchingPaths []string
	var matchingPath string

	for {
		attrs, err := debugIter.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Error listing objects: %v", err)
			break
		}

		// Check if this path contains our update ID
		if strings.Contains(attrs.Name, updateId) {
			matchingPaths = append(matchingPaths, attrs.Name)

			parts := strings.Split(attrs.Name, "/")
			if len(parts) >= 4 && parts[3] == updateId {
				matchingPath = attrs.Name
				log.Printf("Found exact path match: %s", attrs.Name)
			}
		}
	}

	if len(matchingPaths) > 0 {
		log.Printf("Found %d paths containing update ID: %s", len(matchingPaths), updateId)
		for i, path := range matchingPaths {
			log.Printf("  Path %d: %s", i+1, path)
		}
	} else {
		log.Printf("No paths found containing update ID: %s", updateId)
		return
	}

	// If we have a matching path, extract branch and runtime version
	if matchingPath != "" {
		parts := strings.Split(matchingPath, "/")
		if len(parts) >= 4 {
			branch := parts[1]
			runtimeVersion := parts[2]

			log.Printf("Extracted branch: %s, runtimeVersion: %s from path", branch, runtimeVersion)

			// Try to read metadata directly
			metadataPath := path.Join("updates", branch, runtimeVersion, updateId, "metadata.json")
			log.Printf("Attempting to read metadata from: %s", metadataPath)

			reader, err := b.bucket.Object(metadataPath).NewReader(ctx)
			if err != nil {
				log.Printf("Error opening metadata.json: %v", err)

				// Try update-metadata.json as fallback
				fallbackPath := path.Join("updates", branch, runtimeVersion, updateId, "update-metadata.json")
				log.Printf("Trying fallback path: %s", fallbackPath)

				reader, err = b.bucket.Object(fallbackPath).NewReader(ctx)
				if err != nil {
					log.Printf("Error opening update-metadata.json: %v", err)
					return
				}
			}

			// Read and parse the content
			defer reader.Close()
			content, err := io.ReadAll(reader)
			if err != nil {
				log.Printf("Error reading metadata content: %v", err)
				return
			}

			// Pretty print the JSON
			var prettyJSON bytes.Buffer
			err = json.Indent(&prettyJSON, content, "", "  ")
			if err != nil {
				log.Printf("Error formatting JSON: %v", err)
				log.Printf("Raw metadata content: %s", string(content))
			} else {
				log.Printf("Complete metadata for update %s:\n%s", updateId, prettyJSON.String())
			}

			// Also try to parse and extract specific fields
			var metadata map[string]interface{}
			if err := json.Unmarshal(content, &metadata); err == nil {
				log.Printf("Metadata root keys: %v", getMapKeys(metadata))

				// Try to access extra directly
				if extra, ok := metadata["extra"].(map[string]interface{}); ok {
					log.Printf("extra fields: %v", getMapKeys(extra))
					if updateCode, ok := extra["updateCode"]; ok {
						log.Printf("Found extra.updateCode = %v", updateCode)
					} else {
						log.Printf("extra.updateCode not found")
					}

					if buildNumber, ok := extra["buildNumber"]; ok {
						log.Printf("Found extra.buildNumber = %v", buildNumber)
					} else {
						log.Printf("extra.buildNumber not found")
					}
				} else {
					log.Printf("No 'extra' field found at root level")
				}

				// Try to access nested metadata
				if metadataObj, ok := metadata["metadata"].(map[string]interface{}); ok {
					log.Printf("Found nested 'metadata' object with keys: %v", getMapKeys(metadataObj))

					if extra, ok := metadataObj["extra"].(map[string]interface{}); ok {
						log.Printf("metadata.extra fields: %v", getMapKeys(extra))
						if updateCode, ok := extra["updateCode"]; ok {
							log.Printf("Found metadata.extra.updateCode = %v", updateCode)
						} else {
							log.Printf("metadata.extra.updateCode not found")
						}

						if buildNumber, ok := extra["buildNumber"]; ok {
							log.Printf("Found metadata.extra.buildNumber = %v", buildNumber)
						} else {
							log.Printf("metadata.extra.buildNumber not found")
						}
					} else {
						log.Printf("No 'extra' field found in nested metadata")
					}
				} else {
					log.Printf("No nested 'metadata' object found")
				}
			} else {
				log.Printf("Error parsing metadata JSON: %v", err)
			}
		}
	}
}
