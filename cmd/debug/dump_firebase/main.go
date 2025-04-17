package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"expo-open-ota/config"
	"expo-open-ota/internal/types"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"sort"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func main() {
	ctx := context.Background()

	// Set credentials if they're provided as arguments
	if len(os.Args) > 1 {
		os.Setenv("FIREBASE_SERVICE_ACCOUNT", os.Args[1])
	}
	if len(os.Args) > 2 {
		os.Setenv("FIREBASE_STORAGE_BUCKET", os.Args[2])
	}

	// Get Firebase credentials from environment variables
	base64Credentials := config.GetEnv("FIREBASE_SERVICE_ACCOUNT")
	if base64Credentials == "" {
		log.Fatalf("FIREBASE_SERVICE_ACCOUNT environment variable is not set")
	}

	// Decode base64 credentials
	credentials, err := base64.StdEncoding.DecodeString(base64Credentials)
	if err != nil {
		log.Fatalf("error decoding Firebase credentials: %v", err)
	}

	client, err := storage.NewClient(ctx, option.WithCredentialsJSON(credentials))
	if err != nil {
		log.Fatalf("error creating Firebase Storage client: %v", err)
	}
	defer client.Close()

	bucketName := config.GetEnv("FIREBASE_STORAGE_BUCKET")
	if bucketName == "" {
		log.Fatalf("FIREBASE_STORAGE_BUCKET environment variable is not set")
	}

	fmt.Printf("Analyzing Firebase bucket: %s\n\n", bucketName)
	bucket := client.Bucket(bucketName)

	// First, get all branches
	branches, err := getBranches(ctx, bucket)
	if err != nil {
		log.Fatalf("Error getting branches: %v", err)
	}

	fmt.Printf("Found %d branches:\n", len(branches))
	for _, branch := range branches {
		fmt.Printf("  - %s\n", branch)
	}
	fmt.Println()

	// For each branch, get runtime versions
	for _, branch := range branches {
		fmt.Printf("Branch: %s\n", branch)
		runtimeVersions, err := getRuntimeVersions(ctx, bucket, branch)
		if err != nil {
			log.Printf("  Error getting runtime versions: %v", err)
			continue
		}

		fmt.Printf("  Found %d runtime versions:\n", len(runtimeVersions))
		for _, rv := range runtimeVersions {
			fmt.Printf("    - %s\n", rv)
		}
		fmt.Println()

		// For each runtime version, get updates
		for _, rv := range runtimeVersions {
			fmt.Printf("  Runtime Version: %s\n", rv)
			updates, err := getUpdates(ctx, bucket, branch, rv)
			if err != nil {
				log.Printf("    Error getting updates: %v", err)
				continue
			}

			fmt.Printf("    Found %d updates:\n", len(updates))
			for _, update := range updates {
				fmt.Printf("      - %s\n", update.UpdateId)

				// Check for metadata.json
				metadataPath := path.Join("updates", branch, rv, update.UpdateId, "metadata.json")
				metadata, err := checkFileExists(ctx, bucket, metadataPath)
				if err != nil {
					fmt.Printf("        metadata.json: ERROR - %v\n", err)
				} else {
					fmt.Printf("        metadata.json: %t\n", metadata)

					// If metadata exists, try to read it
					if metadata {
						reader, err := bucket.Object(metadataPath).NewReader(ctx)
						if err != nil {
							fmt.Printf("        Error reading metadata.json: %v\n", err)
						} else {
							defer reader.Close()
							content, err := io.ReadAll(reader)
							if err != nil {
								fmt.Printf("        Error reading metadata content: %v\n", err)
							} else {
								var metadataObj map[string]interface{}
								if err := json.Unmarshal(content, &metadataObj); err != nil {
									fmt.Printf("        Error parsing metadata: %v\n", err)
								} else {
									if extra, ok := metadataObj["extra"].(map[string]interface{}); ok {
										buildNumber, hasBuildNumber := extra["buildNumber"]
										updateCode, hasUpdateCode := extra["updateCode"]
										fmt.Printf("        extra.buildNumber: %v (exists: %t)\n", buildNumber, hasBuildNumber)
										fmt.Printf("        extra.updateCode: %v (exists: %t)\n", updateCode, hasUpdateCode)
									}
								}
							}
						}
					}
				}

				// Check for necessary files
				checkFiles := []string{".check", "update-metadata.json"}
				for _, file := range checkFiles {
					filePath := path.Join("updates", branch, rv, update.UpdateId, file)
					exists, err := checkFileExists(ctx, bucket, filePath)
					if err != nil {
						fmt.Printf("        %s: ERROR - %v\n", file, err)
					} else {
						fmt.Printf("        %s: %t\n", file, exists)
					}
				}
			}
			fmt.Println()
		}
	}
}

func getBranches(ctx context.Context, bucket *storage.BucketHandle) ([]string, error) {
	prefix := "updates/"
	query := &storage.Query{
		Prefix:    prefix,
		Delimiter: "/",
	}
	iter := bucket.Objects(ctx, query)

	var branches []string
	for {
		attrs, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error iterating objects: %w", err)
		}

		if attrs.Prefix != "" {
			branch := strings.TrimPrefix(attrs.Prefix, prefix)
			branch = strings.TrimSuffix(branch, "/")
			if branch != "" {
				branches = append(branches, branch)
			}
		}
	}

	sort.Strings(branches)
	return branches, nil
}

func getRuntimeVersions(ctx context.Context, bucket *storage.BucketHandle, branch string) ([]string, error) {
	prefix := path.Join("updates", branch) + "/"
	query := &storage.Query{
		Prefix:    prefix,
		Delimiter: "/",
	}
	iter := bucket.Objects(ctx, query)

	var runtimeVersions []string
	for {
		attrs, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error iterating objects: %w", err)
		}

		if attrs.Prefix != "" {
			version := strings.TrimPrefix(attrs.Prefix, prefix)
			version = strings.TrimSuffix(version, "/")
			if version != "" {
				runtimeVersions = append(runtimeVersions, version)
			}
		}
	}

	sort.Strings(runtimeVersions)
	return runtimeVersions, nil
}

func getUpdates(ctx context.Context, bucket *storage.BucketHandle, branch string, runtimeVersion string) ([]types.Update, error) {
	var result []types.Update
	objectPath := path.Join("updates", branch, runtimeVersion)

	// List all objects in the specified path
	iter := bucket.Objects(ctx, &storage.Query{
		Prefix:    objectPath,
		Delimiter: "/",
	})

	var paths []string
	for {
		attrs, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		// For directory prefixes, extract the update ID
		if attrs.Prefix != "" {
			updateID := path.Base(strings.TrimSuffix(attrs.Prefix, "/"))
			paths = append(paths, updateID)
		}
	}

	for _, updateID := range paths {
		update := types.Update{
			Branch:         branch,
			RuntimeVersion: runtimeVersion,
			UpdateId:       updateID,
		}
		result = append(result, update)
	}

	return result, nil
}

func checkFileExists(ctx context.Context, bucket *storage.BucketHandle, path string) (bool, error) {
	_, err := bucket.Object(path).Attrs(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
