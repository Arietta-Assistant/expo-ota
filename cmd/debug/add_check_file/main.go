package main

import (
	"context"
	"encoding/base64"
	"expo-open-ota/config"
	"fmt"
	"log"
	"os"
	"path"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: go run cmd/debug/add_check_file/main.go [branch] [runtimeVersion] [updateId]")
		fmt.Println("Example: go run cmd/debug/add_check_file/main.go ota-updates 1.0.2 my-update-id")
		os.Exit(1)
	}

	branch := os.Args[1]
	runtimeVersion := os.Args[2]
	updateId := os.Args[3]

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

	fmt.Printf("Adding .check file to update in bucket %s\n", bucketName)
	bucket := client.Bucket(bucketName)

	// Check if update exists
	updatePath := path.Join("updates", branch, runtimeVersion, updateId)
	metadataPath := path.Join(updatePath, "metadata.json")

	_, err = bucket.Object(metadataPath).Attrs(ctx)
	if err != nil {
		log.Fatalf("Error: Update %s/%s/%s does not exist or metadata.json is missing: %v",
			branch, runtimeVersion, updateId, err)
	}

	// Add .check file
	checkFilePath := path.Join(updatePath, ".check")
	writer := bucket.Object(checkFilePath).NewWriter(ctx)
	writer.ContentType = "text/plain"

	_, err = writer.Write([]byte(".check"))
	if err != nil {
		log.Fatalf("Error writing .check file: %v", err)
	}

	if err := writer.Close(); err != nil {
		log.Fatalf("Error closing writer: %v", err)
	}

	fmt.Printf("Successfully added .check file to update %s/%s/%s\n", branch, runtimeVersion, updateId)
	fmt.Println("This update should now be considered valid by the system.")
}
