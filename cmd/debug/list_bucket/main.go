package main

import (
	"context"
	"encoding/base64"
	"expo-open-ota/config"
	"fmt"
	"log"
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

	fmt.Printf("Listing contents of bucket: %s\n", bucketName)
	bucket := client.Bucket(bucketName)

	// List all objects in the bucket
	query := &storage.Query{
		Prefix: "updates/",
	}

	iter := bucket.Objects(ctx, query)
	fmt.Println("Listing all paths in the bucket:")

	// Track unique branches
	branches := make(map[string]bool)

	for {
		attrs, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("Error listing objects: %v", err)
		}

		fmt.Println(attrs.Name)

		// Extract branch
		if strings.HasPrefix(attrs.Name, "updates/") {
			parts := strings.Split(attrs.Name, "/")
			if len(parts) >= 2 {
				branch := parts[1]
				branches[branch] = true
			}
		}
	}

	fmt.Println("\nDetected branches:")
	for branch := range branches {
		fmt.Println("- " + branch)
	}
}
