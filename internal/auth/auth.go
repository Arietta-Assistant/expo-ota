package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"expo-open-ota/config"
	"expo-open-ota/internal/db"
	"expo-open-ota/internal/services"
	"fmt"
	"log"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/api/option"
)

var (
	authClient *auth.Client
)

func init() {
	// Initialize Firebase Admin SDK
	encodedServiceAccount := config.GetEnv("FIREBASE_SERVICE_ACCOUNT")
	serviceAccount, err := base64.StdEncoding.DecodeString(encodedServiceAccount)
	if err != nil {
		log.Fatalf("Error decoding Firebase service account: %v", err)
	}

	opt := option.WithCredentialsJSON(serviceAccount)
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		log.Fatalf("Error initializing Firebase app: %v", err)
	}

	client, err := app.Auth(context.Background())
	if err != nil {
		log.Fatalf("Error getting Auth client: %v", err)
	}

	authClient = client
}

type Auth struct {
	Secret string
}

func getAdminPassword() string {
	return config.GetEnv("ADMIN_PASSWORD")
}

func isPasswordValid(password string) bool {
	adminPassword := getAdminPassword()
	if adminPassword == "" {
		fmt.Errorf("admin password is not set, all requests will be rejected")
		return false
	}
	return password == getAdminPassword()
}

type AuthResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
}

func NewAuth() *Auth {
	return &Auth{Secret: config.GetEnv("JWT_SECRET")}
}

func (a *Auth) generateAuthToken() (*string, error) {
	token, err := services.GenerateJWTToken(a.Secret, jwt.MapClaims{
		"sub":  "admin-dashboard",
		"exp":  time.Now().Add(time.Hour * 2).Unix(),
		"iat":  time.Now().Unix(),
		"type": "token",
	})
	if err != nil {
		return nil, fmt.Errorf("error while generating the jwt token: %w", err)
	}
	return &token, nil
}

func (a *Auth) generateRefreshToken() (*string, error) {
	refreshToken, err := services.GenerateJWTToken(a.Secret, jwt.MapClaims{
		"sub":  "admin-dashboard",
		"exp":  time.Now().Add(time.Hour * 24 * 7).Unix(),
		"iat":  time.Now().Unix(),
		"type": "refreshToken",
	})
	if err != nil {
		return nil, fmt.Errorf("error while generating the jwt token: %w", err)
	}
	return &refreshToken, nil
}

func (a *Auth) LoginWithPassword(password string) (*AuthResponse, error) {
	if !isPasswordValid(password) {
		return nil, errors.New("invalid password")
	}
	token, err := a.generateAuthToken()
	if err != nil {
		return nil, err
	}
	refreshToken, err := a.generateRefreshToken()
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		Token:        *token,
		RefreshToken: *refreshToken,
	}, nil
}

func (a *Auth) ValidateToken(tokenString string) (*jwt.Token, error) {
	claims := jwt.MapClaims{}
	token, err := services.DecodeAndExtractJWTToken(a.Secret, tokenString, &claims)
	if err != nil {
		return nil, err
	}
	if claims["type"] != "token" {
		return nil, errors.New("invalid token type")
	}
	if claims["sub"] != "admin-dashboard" {
		return nil, errors.New("invalid token subject")
	}
	return token, nil
}

func (a *Auth) RefreshToken(tokenString string) (*AuthResponse, error) {
	claims := jwt.MapClaims{}
	_, err := services.DecodeAndExtractJWTToken(a.Secret, tokenString, &claims)
	if err != nil {
		return nil, err
	}
	if claims["type"] != "refreshToken" {
		return nil, errors.New("invalid token type")
	}
	if claims["sub"] != "admin-dashboard" {
		return nil, errors.New("invalid token subject")
	}
	newToken, err := a.generateAuthToken()
	if err != nil {
		return nil, err
	}
	refreshToken, err := a.generateRefreshToken()
	if err != nil {
		return nil, err
	}
	return &AuthResponse{
		Token:        *newToken,
		RefreshToken: *refreshToken,
	}, nil
}

// VerifyFirebaseToken verifies a Firebase authentication token and returns user info
func VerifyFirebaseToken(token string) (*auth.Token, error) {
	if token == "" {
		return nil, nil
	}

	// Verify the ID token
	decodedToken, err := authClient.VerifyIDToken(context.Background(), token)
	if err != nil {
		log.Printf("Error verifying Firebase token: %v", err)
		return nil, err
	}

	// Track user access in database
	go trackUserAccess(decodedToken)

	return decodedToken, nil
}

// trackUserAccess records user access in the database
func trackUserAccess(token *auth.Token) {
	// Get user info from token
	userID := token.UID
	email := token.Claims["email"].(string)
	name := token.Claims["name"].(string)
	if name == "" {
		name = email
	}

	// Create or update user record
	user := db.User{
		ID:          userID,
		Email:       email,
		Name:        name,
		LastSeen:    time.Now(),
		UpdateCount: 1, // Increment update count
	}

	err := db.UpsertUser(user)
	if err != nil {
		log.Printf("Error tracking user access: %v", err)
	}
}
