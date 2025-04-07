package config

import (
	"os"
)

// GetEnv returns the value of the environment variable or the default value if not set
func GetEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		switch key {
		case "PORT":
			return "8080"
		case "BUCKET_TYPE":
			return "firebase"
		case "MODULAR_WEB_API_KEY":
			return "" // Don't set a default for sensitive credentials
		case "MODULAR_WEB_API_SECRET":
			return "" // Don't set a default for sensitive credentials
		case "FIREBASE_STORAGE_BUCKET":
			return "" // Don't set a default for Firebase config
		case "FIREBASE_AUTH_DOMAIN":
			return "" // Don't set a default for Firebase config
		case "FIREBASE_PROJECT_ID":
			return "" // Don't set a default for Firebase config
		case "FIREBASE_APP_ID":
			return "" // Don't set a default for Firebase config
		case "FIREBASE_MEASUREMENT_ID":
			return "" // Don't set a default for Firebase config
		case "FIREBASE_MESSAGING_SENDER_ID":
			return "" // Don't set a default for Firebase config
		}
	}
	return value
}

// GetModularWebCredentials returns the Modular Web API credentials
func GetModularWebCredentials() (string, string) {
	key := GetEnv("MODULAR_WEB_API_KEY")
	secret := GetEnv("MODULAR_WEB_API_SECRET")
	return key, secret
}

// GetFirebaseConfig returns all Firebase configuration values
func GetFirebaseConfig() map[string]string {
	return map[string]string{
		"storageBucket":     GetEnv("FIREBASE_STORAGE_BUCKET"),
		"authDomain":        GetEnv("FIREBASE_AUTH_DOMAIN"),
		"projectId":         GetEnv("FIREBASE_PROJECT_ID"),
		"appId":             GetEnv("FIREBASE_APP_ID"),
		"measurementId":     GetEnv("FIREBASE_MEASUREMENT_ID"),
		"messagingSenderId": GetEnv("FIREBASE_MESSAGING_SENDER_ID"),
	}
}
