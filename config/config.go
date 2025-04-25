package config

import (
	"expo-open-ota/internal/helpers"
	"flag"
	"log"
	"os"

	"github.com/joho/godotenv"
)

func validateStorageMode(storageMode string) bool {
	return storageMode == "local" || storageMode == "s3" || storageMode == "firebase"
}

func validateBucketParams(storageMode string) bool {
	switch storageMode {
	case "s3":
		bucketName := GetEnv("S3_BUCKET_NAME")
		if bucketName == "" {
			log.Printf("S3_BUCKET_NAME not set")
			return false
		}
		region := GetEnv("AWS_REGION")
		if region == "" {
			log.Printf("AWS_REGION not set")
			return false
		}
	case "firebase":
		// Check for Firebase project ID or service account credentials
		projectID := GetEnv("FIREBASE_PROJECT_ID")
		serviceAccount := GetEnv("FIREBASE_SERVICE_ACCOUNT")
		if projectID == "" && serviceAccount == "" {
			log.Printf("Neither FIREBASE_PROJECT_ID nor FIREBASE_SERVICE_ACCOUNT is set")
			return false
		}
		// Bucket name is optional (derived from project ID if not set)
		// but we'll log a warning if it's missing
		bucketName := GetEnv("FIREBASE_STORAGE_BUCKET")
		if bucketName == "" && projectID != "" {
			log.Printf("FIREBASE_STORAGE_BUCKET not set, will use default from project ID: %s.appspot.com", projectID)
		} else if bucketName == "" {
			log.Printf("Warning: FIREBASE_STORAGE_BUCKET not set and cannot be derived (no project ID)")
		}
		return true
	case "local":
		// Already handled by default values
		return true
	default:
		return false
	}
	return true
}

func validateBaseUrl(baseUrl string) bool {
	return baseUrl != "" && helpers.IsValidURL(baseUrl)
}

func IsTestMode() bool {
	return flag.Lookup("test.v") != nil
}

func LoadConfig() {
	err := godotenv.Load()
	if err != nil {
		log.Printf("No .env file found, continuing with runtime environment variables.")
	}
	storageMode := GetEnv("STORAGE_MODE")
	if !validateStorageMode(storageMode) {
		log.Fatalf("Invalid STORAGE_MODE: %s", storageMode)
	}
	bucketParamsValid := validateBucketParams(storageMode)
	if !bucketParamsValid {
		log.Fatalf("Invalid bucket parameters")
	}
	baseUrl := GetEnv("BASE_URL")
	if !validateBaseUrl(baseUrl) {
		log.Fatalf("Invalid BASE_URL: %s", baseUrl)
	}
	expoToken := GetEnv("EXPO_ACCESS_TOKEN")
	if expoToken == "" {
		log.Println("Warning: EXPO_ACCESS_TOKEN not set. Some features may be limited.")
	}
	expoAppId := GetEnv("EXPO_APP_ID")
	if expoAppId == "" {
		log.Fatalf("EXPO_APP_ID not set")
	}
	jwtSecret := GetEnv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatalf("JWT_SECRET not set")
	}
}

var DefaultEnvValues = map[string]string{
	"LOCAL_BUCKET_BASE_PATH":      "./updates",
	"STORAGE_MODE":                "local",
	"BUCKET_TYPE":                 "local",
	"BASE_URL":                    "http://localhost:3000",
	"PUBLIC_LOCAL_EXPO_KEY_PATH":  "./keyStore/public-key.pem",
	"PRIVATE_LOCAL_EXPO_KEY_PATH": "./keyStore/private-key.pem",
	"KEYS_STORAGE_TYPE":           "local",
	"JWT_SECRET":                  "",
	"AWS_REGION":                  "eu-west-3",
	"FIREBASE_PROJECT_ID":         "",
	"FIREBASE_STORAGE_BUCKET":     "",
	"FIREBASE_SERVICE_ACCOUNT":    "",
}

func GetEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		defaultValue := DefaultEnvValues[key]
		if defaultValue != "" {
			return defaultValue
		}
		return ""
	}
	return value
}
