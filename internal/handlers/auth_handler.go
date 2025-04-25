package handlers

import (
	"expo-open-ota/config"
	"expo-open-ota/internal/auth"
	"expo-open-ota/internal/dashboard"
	"log"
	"net/http"
)

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Login request received")
	dashboardEnabled := dashboard.IsDashboardEnabled()
	if !dashboardEnabled {
		log.Println("Login failed: dashboard not enabled")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	password := r.FormValue("password")
	if password == "" {
		log.Println("Login failed: no password provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Println("Authenticating with password...")
	adminPassword := config.GetEnv("ADMIN_PASSWORD")
	if adminPassword == "" {
		log.Println("Login failed: ADMIN_PASSWORD not configured")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	authService := auth.NewAuth()
	authResponse, err := authService.LoginWithPassword(password)
	if err != nil {
		log.Printf("Login failed: %v", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	log.Println("Login successful, returning tokens")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"token":"` + authResponse.Token + `","refreshToken":"` + authResponse.RefreshToken + `"}`))
}

func RefreshTokenHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Refresh token request received")
	dashboardEnabled := dashboard.IsDashboardEnabled()
	if !dashboardEnabled {
		log.Println("Refresh token failed: dashboard not enabled")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	refreshToken := r.FormValue("refreshToken")
	if refreshToken == "" {
		log.Println("Refresh token failed: no refresh token provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	authService := auth.NewAuth()
	authResponse, err := authService.RefreshToken(refreshToken)
	if err != nil {
		log.Printf("Refresh token failed: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Println("Token refresh successful, returning new tokens")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"token":"` + authResponse.Token + `","refreshToken":"` + authResponse.RefreshToken + `"}`))
}
