package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"mini-shop-backend/auth"
	"mini-shop-backend/repository"
)

type AuthHandler struct{ Users *repository.UserRepository }

func NewAuthHandler(users *repository.UserRepository) *AuthHandler { return &AuthHandler{Users: users} }

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

type verifyOTPRequest struct {
	Mobile string `json:"mobile_number"`
	OTP    string `json:"otp"`
}

// VerifyOTP is the first step for both returning and new users.
func (h *AuthHandler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	var req verifyOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}
	req.Mobile = strings.TrimSpace(req.Mobile)
	if !validMobile(req.Mobile) || req.OTP != "0000" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Enter a valid OTP(0000)"})
		return
	}

	user, err := h.Users.FindByMobile(r.Context(), req.Mobile)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("Phone verification lookup error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not verify phone number"})
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		onboardingToken, tokenErr := auth.GenerateOnboardingToken(req.Mobile)
		if tokenErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not create verification session"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"needs_profile": true, "onboarding_token": onboardingToken})
		return
	}
	if !user.IsVerified {
		if err := h.Users.VerifyUser(r.Context(), user.ID); err != nil {
			log.Printf("Phone verification update error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not verify account"})
			return
		}
		user.IsVerified = true
	}
	token, err := auth.GenerateToken(user.ID, user.Role)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not create authentication token"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": user})
}

type registerRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// Register completes the profile step after the OTP endpoint issued an onboarding token.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Phone verification is required"})
		return
	}
	mobile, err := auth.ParseOnboardingToken(parts[1])
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Phone verification expired; please verify again"})
		return
	}
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	req.Role = strings.ToLower(strings.TrimSpace(req.Role))
	if req.Name == "" || len([]rune(req.Name)) > 20 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Username is required and must be at most 20 characters"})
		return
	}
	if len(req.Email) > 254 || (req.Email != "" && (!strings.Contains(req.Email, "@") || strings.ContainsAny(req.Email, " \t\r\n"))) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Enter a valid email address"})
		return
	}
	if req.Role != "seller" && req.Role != "customer" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Account type must be seller or customer"})
		return
	}
	user, err := h.Users.CreateVerified(r.Context(), req.Name, mobile, req.Email, req.Role)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Mobile number is already registered"})
			return
		}
		log.Printf("Account creation error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not create account"})
		return
	}
	token, err := auth.GenerateToken(user.ID, user.Role)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not create authentication token"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "user": user})
}

func validMobile(mobile string) bool {
	if len(mobile) < 10 || len(mobile) > 12 {
		return false
	}
	for _, ch := range mobile {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
