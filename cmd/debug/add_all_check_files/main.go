package main

import (
	"context"
	"encoding/base64"
	"expo-open-ota/config"
	"expo-open-ota/internal/types"
	"fmt"
	"log"
	"path"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func main() {
	ctx := context.Background()

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

	fmt.Printf("Adding .check files to all updates in bucket: %s\n", bucketName)
	bucket := client.Bucket(bucketName)

	// Get all branches
	branches, err := getBranches(ctx, bucket)
	if err != nil {
		log.Fatalf("Error getting branches: %v", err)
	}

	totalUpdates := 0
	totalFixed := 0

	// Process each branch
	for _, branch := range branches {
		fmt.Printf("\nBranch: %s\n", branch)

		// Get runtime versions for this branch
		runtimeVersions, err := getRuntimeVersions(ctx, bucket, branch)
		if err != nil {
			fmt.Printf("  Error getting runtime versions: %v\n", err)
			continue
		}

		// Process each runtime version
		for _, rv := range runtimeVersions {
			fmt.Printf("  Runtime Version: %s\n", rv)

			// Get updates for this runtime version
			updates, err := getUpdates(ctx, bucket, branch, rv)
			if err != nil {
				fmt.Printf("    Error getting updates: %v\n", err)
				continue
			}

			totalUpdates += len(updates)

			// Process each update
			for _, update := range updates {
				fmt.Printf("    Update: %s\n", update.UpdateId)

				// Check if .check file exists
				checkPath := path.Join("updates", branch, rv, update.UpdateId, ".check")
				exists, err := fileExists(ctx, bucket, checkPath)
				if err != nil {
					fmt.Printf("      Error checking if .check exists: %v\n", err)
					continue
				}

				if exists {
					fmt.Printf("      .check file already exists\n")
					continue
				}

				// Check if metadata.json exists (required for a valid update)
				metadataPath := path.Join("updates", branch, rv, update.UpdateId, "metadata.json")
				metadataExists, err := fileExists(ctx, bucket, metadataPath)
				if err != nil {
					fmt.Printf("      Error checking if metadata.json exists: %v\n", err)
					continue
				}

				if !metadataExists {
					fmt.Printf("      metadata.json doesn't exist, skipping\n")
					continue
				}

				// Add .check file
				err = addCheckFile(ctx, bucket, branch, rv, update.UpdateId)
				if err != nil {
					fmt.Printf("      Error adding .check file: %v\n", err)
					continue
				}

				fmt.Printf("      Successfully added .check file\n")
				totalFixed++
			}
		}
	}

	fmt.Printf("\nSummary: Added .check files to %d out of %d total updates\n", totalFixed, totalUpdates)
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

func fileExists(ctx context.Context, bucket *storage.BucketHandle, path string) (bool, error) {
	_, err := bucket.Object(path).Attrs(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func addCheckFile(ctx context.Context, bucket *storage.BucketHandle, branch, runtimeVersion, updateId string) error {
	checkFilePath := path.Join("updates", branch, runtimeVersion, updateId, ".check")
	writer := bucket.Object(checkFilePath).NewWriter(ctx)
	writer.ContentType = "text/plain"

	_, err := writer.Write([]byte(".check"))
	if err != nil {
		return err
	}

	return writer.Close()
}
