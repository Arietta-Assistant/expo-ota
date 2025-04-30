package usertracking

import (
	"log"
	"time"

	"expo-open-ota/internal/bucket"
	"expo-open-ota/internal/types"
)

// TrackAssetDownload records a user's download of an update asset
func TrackAssetDownload(branch, runtimeVersion, updateId, platform, firebaseToken, deviceId string) {
	log.Printf("Tracking asset download: branch=%s, runtime=%s, updateId=%s, platform=%s",
		branch, runtimeVersion, updateId, platform)

	// If device ID is not provided, use a default
	if deviceId == "" {
		deviceId = "unknown-device"
	}

	// Get the bucket storage
	storage := bucket.GetBucket()

	// Try to extract a user ID from the firebase token
	// For now this is a simple approach - we should use Firebase Admin SDK to verify tokens in production
	userId := extractUserIdFromToken(firebaseToken)

	// Record download information
	download := types.UpdateDownload{
		UpdateId:       updateId,
		UserId:         userId,
		DeviceId:       deviceId,
		Platform:       platform,
		DownloadedAt:   time.Now().Format(time.RFC3339),
		RuntimeVersion: runtimeVersion,
		Branch:         branch,
	}

	// Store the download record, with timeout protection
	done := make(chan bool, 1)
	go func() {
		err := storage.StoreUpdateDownload(download)
		if err != nil {
			log.Printf("Error storing update download: %v", err)
		} else {
			log.Printf("Successfully recorded download for user %s, update %s", userId, updateId)
		}
		done <- true
	}()

	// Use a timeout to prevent hanging
	select {
	case <-done:
		// Storage operation completed successfully
	case <-time.After(5 * time.Second):
		log.Printf("Warning: Update download tracking timed out after 5 seconds")
	}
}

// extractUserIdFromToken attempts to extract a user ID from a Firebase token
// In a production environment, this should use Firebase Admin SDK to verify and decode the token
func extractUserIdFromToken(token string) string {
	if token == "" {
		return "anonymous"
	}

	// For demo purposes, we'll just use a hash of the token
	// In production, use Firebase Admin SDK to properly verify and decode
	return "user-" + hashString(token)
}

// Simple hash function for demo purposes
func hashString(s string) string {
	// Take a portion of the token as a simple ID
	// This is NOT secure - just for demonstration
	if len(s) > 10 {
		return s[len(s)-10:]
	}
	return s
}
