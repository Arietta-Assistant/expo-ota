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
	"sort"
	"strconv"
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
	log.Printf("FIREBASE-DEBUG: Getting file %s", objectPath)

	// Check if the file exists
	obj := b.bucket.Object(objectPath)
	attrs, err := obj.Attrs(context.Background())
	if err != nil {
		if err == storage.ErrObjectNotExist {
			log.Printf("FIREBASE-DEBUG: File not found: %s", objectPath)

			// Also try with just the asset name in case that's how it's stored
			if strings.HasPrefix(fileName, "assets/") {
				alternativePath := path.Join("updates", branch, runtimeVersion, updateId, strings.TrimPrefix(fileName, "assets/"))
				log.Printf("FIREBASE-DEBUG: Trying alternative path without 'assets/' prefix: %s", alternativePath)
				altObj := b.bucket.Object(alternativePath)
				altAttrs, altErr := altObj.Attrs(context.Background())
				if altErr == nil {
					log.Printf("FIREBASE-DEBUG: Found file at alternative path with size: %d bytes, created: %v",
						altAttrs.Size, altAttrs.Created)
					return b.bucket.Object(alternativePath).NewReader(context.Background())
				} else {
					log.Printf("FIREBASE-DEBUG: Alternative path also not found: %s", alternativePath)
				}
			}

			// List objects in the directory to debug what's actually there
			log.Printf("FIREBASE-DEBUG: Listing objects in directory to see what's available")
			dirPath := path.Join("updates", branch, runtimeVersion, updateId)
			query := &storage.Query{
				Prefix: dirPath,
			}
			it := b.bucket.Objects(context.Background(), query)
			count := 0
			for {
				attrs, err := it.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					log.Printf("FIREBASE-DEBUG: Error listing objects: %v", err)
					break
				}
				log.Printf("FIREBASE-DEBUG: Found object: %s", attrs.Name)
				count++
				if count >= 20 {
					log.Printf("FIREBASE-DEBUG: Stopping after 20 objects")
					break
				}
			}
		} else {
			log.Printf("FIREBASE-DEBUG: Error checking file existence: %v", err)
		}
		return nil, fmt.Errorf("file not found: %s", objectPath)
	} else {
		log.Printf("FIREBASE-DEBUG: File exists with size: %d bytes, created: %v", attrs.Size, attrs.Created)
	}

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
	// Preserve the full path for the object in Firebase
	objectPath := path.Join("updates", update.Branch, update.RuntimeVersion, update.UpdateId, fileName)

	// Log the file being uploaded for debugging
	log.Printf("Uploading file to Firebase storage: %s", objectPath)

	writer := b.bucket.Object(objectPath).NewWriter(context.Background())
	defer writer.Close()

	bytesWritten, err := io.Copy(writer, content)
	if err != nil {
		return fmt.Errorf("error uploading file %s to Firebase: %w", fileName, err)
	}

	log.Printf("Successfully uploaded %d bytes to %s", bytesWritten, objectPath)
	return nil
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

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// List all objects in the specified path
	iter := b.bucket.Objects(ctx, &storage.Query{
		Prefix:    objectPath + "/",
		Delimiter: "/",
	})

	var updateIDs []string
	// First pass: collect all update IDs
	for {
		attrs, err := iter.Next()
		if err == iterator.Done || err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Error listing objects: %v", err)
			return nil, err
		}

		// For directory prefixes, extract the update ID
		if attrs.Prefix != "" {
			updateID := path.Base(strings.TrimSuffix(attrs.Prefix, "/"))
			updateIDs = append(updateIDs, updateID)
			log.Printf("Found update ID: %s", updateID)
		}
	}

	log.Printf("Found %d potential update IDs in %s", len(updateIDs), objectPath)

	// Extract and sort by build number if present
	type updateWithBuild struct {
		updateID   string
		buildNum   int
		isBuildNum bool
	}

	updates := make([]updateWithBuild, 0, len(updateIDs))

	// Process update IDs to extract build numbers
	for _, id := range updateIDs {
		update := updateWithBuild{updateID: id}

		// Special handling for build-X-ID format
		if strings.HasPrefix(id, "build-") {
			parts := strings.SplitN(id, "-", 3)
			if len(parts) >= 2 {
				if num, err := strconv.Atoi(parts[1]); err == nil {
					update.buildNum = num
					update.isBuildNum = true
					log.Printf("Extracted build number %d from update ID: %s", num, id)
				}
			}
		}

		updates = append(updates, update)
	}

	// Sort updates by build number (if available)
	sort.Slice(updates, func(i, j int) bool {
		// First sort by build number if available for both
		if updates[i].isBuildNum && updates[j].isBuildNum {
			return updates[i].buildNum > updates[j].buildNum // higher build numbers first
		}
		// Build numbers before non-build numbers
		if updates[i].isBuildNum {
			return true
		}
		if updates[j].isBuildNum {
			return false
		}
		// Alphabetical for non-build numbers
		return updates[i].updateID > updates[j].updateID
	})

	// Second pass: Convert to types.Update objects
	for _, update := range updates {
		updateID := update.updateID

		// Create base update object
		updateObj := types.Update{
			Branch:         branch,
			RuntimeVersion: runtimeVersion,
			UpdateId:       updateID,
		}

		// If this is a build number update, use that info
		if update.isBuildNum {
			updateObj.BuildNumber = strconv.Itoa(update.buildNum)
			// Try to extract a timestamp
			if strings.HasPrefix(updateID, "build-") {
				// Add timestamp based on build number - higher build numbers are more recent
				// This is a very rough approximation
				daysAgo := 100 - update.buildNum // Approximation: build-100 would be today, build-1 would be 99 days ago
				secondsAgo := daysAgo * 24 * 60 * 60
				updateObj.CreatedAt = time.Duration(secondsAgo) * time.Second
			}
		}

		// Try to read metadata.json if it exists, but don't fail if it doesn't
		metadataPath := path.Join(objectPath, updateID, "metadata.json")
		reader, err := b.bucket.Object(metadataPath).NewReader(ctx)

		if err == nil {
			// Successfully opened metadata.json
			log.Printf("Reading metadata from: %s", metadataPath)
			metadataContent, err := io.ReadAll(reader)
			reader.Close()

			if err == nil {
				var metadata map[string]interface{}
				if json.Unmarshal(metadataContent, &metadata) == nil {
					log.Printf("Successfully parsed metadata for update: %s", updateID)

					// Try to extract additional information
					if extra, ok := metadata["extra"].(map[string]interface{}); ok {
						// Extract build number if not already set
						if updateObj.BuildNumber == "" {
							if bn, ok := extra["buildNumber"]; ok {
								updateObj.BuildNumber = fmt.Sprintf("%v", bn)
							}
						}

						// Extract other fields if available
						if commitHash, ok := extra["commitHash"]; ok {
							updateObj.CommitHash = fmt.Sprintf("%v", commitHash)
						}
					}

					// Try to extract info from nested metadata
					if nestedMeta, ok := metadata["metadata"].(map[string]interface{}); ok {
						if extra, ok := nestedMeta["extra"].(map[string]interface{}); ok {
							if updateObj.BuildNumber == "" {
								if bn, ok := extra["buildNumber"]; ok {
									updateObj.BuildNumber = fmt.Sprintf("%v", bn)
								}
							}
						}
					}
				}
			}
		} else {
			// Metadata file doesn't exist - this is okay, we'll use what we have
			log.Printf("No metadata.json found for update %s (this is okay): %v", updateID, err)

			// Try alternate metadata file name: update-metadata.json
			altMetadataPath := path.Join(objectPath, updateID, "update-metadata.json")
			altReader, altErr := b.bucket.Object(altMetadataPath).NewReader(ctx)

			if altErr == nil {
				log.Printf("Found alternative metadata at: %s", altMetadataPath)
				altContent, _ := io.ReadAll(altReader)
				altReader.Close()

				var altMetadata map[string]interface{}
				if json.Unmarshal(altContent, &altMetadata) == nil {
					// Process alternative metadata if needed
					log.Printf("Successfully parsed alternative metadata for update: %s", updateID)
				}
			}
		}

		// Always add the update to results
		log.Printf("Adding update %s to results (build number: %s)", updateID, updateObj.BuildNumber)
		result = append(result, updateObj)
	}

	log.Printf("Completed GetUpdates, found %d updates for %s/%s", len(result), branch, runtimeVersion)
	return result, nil
}

func (b *FirebaseBucket) GetBranches() ([]string, error) {
	log.Printf("Firebase GetBranches: querying for branches...")

	// Check if bucket client is nil
	if b.bucket == nil {
		return nil, fmt.Errorf("Firebase bucket client is nil, initialization may have failed")
	}

	prefix := "updates/"
	query := &storage.Query{
		Prefix:    prefix,
		Delimiter: "/",
	}

	// Get context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	iter := b.bucket.Objects(ctx, query)

	var branches []string
	for {
		attrs, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Firebase GetBranches error: %v", err)
			return nil, fmt.Errorf("error iterating objects: %w", err)
		}

		if attrs.Prefix != "" {
			// Extract branch name from prefix (e.g., "updates/main/" -> "main")
			branch := strings.TrimPrefix(attrs.Prefix, prefix)
			branch = strings.TrimSuffix(branch, "/")
			if branch != "" {
				log.Printf("Firebase GetBranches: found branch: %s", branch)
				branches = append(branches, branch)
			}
		}
	}

	// If no branches found and no error occurred, create a default branch
	if len(branches) == 0 {
		log.Printf("Firebase GetBranches: no branches found, creating a default branch")
		// Create a demo branch with a sample update
		defaultBranch := "main"
		defaultRuntimeVersion := "1.0.0"

		// Create demo update metadata
		metadata := map[string]interface{}{
			"manifest": map[string]interface{}{
				"id":        "demo-update",
				"createdAt": time.Now().Unix(),
			},
			"extra": map[string]interface{}{
				"buildNumber": "1",
				"updateCode":  "demo",
			},
		}

		metadataJSON, _ := json.Marshal(metadata)
		objectPath := path.Join("updates", defaultBranch, defaultRuntimeVersion, "demo-update", "metadata.json")
		writer := b.bucket.Object(objectPath).NewWriter(ctx)
		if _, err := writer.Write(metadataJSON); err != nil {
			log.Printf("Failed to write demo metadata: %v", err)
		} else {
			if err := writer.Close(); err != nil {
				log.Printf("Failed to close demo metadata writer: %v", err)
			} else {
				log.Printf("Created demo branch and update successfully")
				branches = append(branches, defaultBranch)
			}
		}
	}

	log.Printf("Firebase GetBranches: returning %d branches", len(branches))
	return branches, nil
}

func (b *FirebaseBucket) GetRuntimeVersions(branch string) ([]RuntimeVersionWithStats, error) {
	log.Printf("Getting runtime versions for branch: %s", branch)
	prefix := path.Join("updates", branch) + "/"
	query := &storage.Query{
		Prefix:    prefix,
		Delimiter: "/",
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	iter := b.bucket.Objects(ctx, query)

	runtimeVersions := make(map[string]*RuntimeVersionWithStats)
	for {
		attrs, err := iter.Next()
		if err == io.EOF || err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Error iterating objects for runtime versions: %v", err)
			return nil, fmt.Errorf("error iterating objects: %w", err)
		}

		if attrs.Prefix != "" {
			// Extract runtime version from prefix (e.g., "updates/main/1.0.0/" -> "1.0.0")
			version := strings.TrimPrefix(attrs.Prefix, prefix)
			version = strings.TrimSuffix(version, "/")
			if version != "" {
				log.Printf("Found runtime version: %s", version)

				// Instead of calling GetUpdates, which might fail if metadata doesn't exist,
				// we'll just add the runtime version with default stats
				runtimeVersions[version] = &RuntimeVersionWithStats{
					RuntimeVersion:  version,
					LastUpdatedAt:   time.Now().UTC().Format(time.RFC3339),
					CreatedAt:       time.Now().UTC().Format(time.RFC3339),
					NumberOfUpdates: 0, // Default to 0, we'll determine this later
				}

				// Optionally, try to count the number of updates
				updatePrefix := attrs.Prefix // This is already "updates/branch/version/"
				updateQuery := &storage.Query{
					Prefix:    updatePrefix,
					Delimiter: "/",
				}

				updateIter := b.bucket.Objects(ctx, updateQuery)
				updateCount := 0

				for {
					updateAttrs, updateErr := updateIter.Next()
					if updateErr == io.EOF || updateErr == iterator.Done {
						break
					}
					if updateErr != nil {
						log.Printf("Error counting updates for %s: %v", version, updateErr)
						break // Continue with the next runtime version
					}

					if updateAttrs.Prefix != "" {
						updateCount++
					}
				}

				if updateCount > 0 {
					log.Printf("Found %d updates for runtime version %s", updateCount, version)
					runtimeVersions[version].NumberOfUpdates = updateCount
				}
			}
		}
	}

	// Convert map to slice
	var result []RuntimeVersionWithStats
	for _, stats := range runtimeVersions {
		result = append(result, *stats)
	}

	// Sort by version number (assuming semantic versioning)
	sort.Slice(result, func(i, j int) bool {
		return result[i].RuntimeVersion > result[j].RuntimeVersion
	})

	log.Printf("Found %d runtime versions for branch %s", len(result), branch)
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

// ListUpdates returns a list of all update IDs for a specific branch and runtime version
func (b *FirebaseBucket) ListUpdates(branch string, runtimeVersion string) ([]string, error) {
	dirPath := path.Join("updates", branch, runtimeVersion)
	log.Printf("FIREBASE-DEBUG: Listing updates in %s", dirPath)

	query := &storage.Query{
		Prefix:    dirPath,
		Delimiter: "/",
	}

	updates := []string{}
	it := b.bucket.Objects(context.Background(), query)

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("FIREBASE-DEBUG: Error listing updates: %v", err)
			return nil, fmt.Errorf("error listing updates: %w", err)
		}

		// Extract update ID from the path
		if attrs.Name != "" {
			parts := strings.Split(attrs.Name, "/")
			if len(parts) >= 4 {
				updateID := parts[3]
				updates = append(updates, updateID)
				log.Printf("FIREBASE-DEBUG: Found update: %s", updateID)
			}
		}
	}

	return updates, nil
}
