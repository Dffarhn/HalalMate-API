package controllers

import (
	"HalalMate/services"
	"HalalMate/utils"
	"bytes"
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type GoogleAuthRequest struct {
	IDToken string `json:"idToken"`
}

// AuthHandler struct
type AuthController struct {
	AuthService *services.AuthService
}

// NewAuthHandler initializes AuthHandler
func NewAuthController() *AuthController {
	return &AuthController{
		AuthService: services.NewAuthService(),
	}
}

// RegisterUser handles user registration
func (h *AuthController) RegisterUser(c *gin.Context) {
	var req struct {
		Username        string `json:"username" binding:"required"`
		Email           string `json:"email" binding:"required"`
		Password        string `json:"password" binding:"required"`
		RetypedPassword string `json:"retyped_password" binding:"required"`
		// FCMToken        string `json:"fcm_token"`
	}


	// 🕵️ Read and log the raw body
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Failed to read request body")
		return
	}

	log.Printf("[DEBUG] 🔍 Raw JSON Body: %s", string(bodyBytes))

	// 🧠 Replace the body so it can still be read by ShouldBindJSON
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ERROR] 🔥 Bind error: %v", err)
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request format")
		return
	}

	user, token, err := h.AuthService.Register(req.Email, req.Username, req.Password)
	if err != nil {
		c.Error(err)
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "User registered successfully", gin.H{
		"token": token,
		"user":  user,
	})
}

// LoginUser handles user login
func (h *AuthController) LoginUser(c *gin.Context) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Println("[ERROR] Invalid request format:", err)
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request format")
		return
	}

	log.Printf("[INFO] User login attempt: Email - %s", req.Email)

	token, err := h.AuthService.Login(req.Email, req.Password)
	if err != nil {
		log.Println("[ERROR] Login failed:", err)
		utils.ErrorResponse(c, http.StatusUnauthorized, err.Error())
		return
	}

	log.Println("[INFO] Login successful, token generated")

	utils.SuccessResponse(c, http.StatusOK, "Login successful", gin.H{
		"token": token,
	})
}

// made it when use google idtoken
// GoogleLogin handles Google login verification
func (h *AuthController) GoogleLogin(c *gin.Context) {
	var req struct {
		IDToken string `json:"idToken"`
	}

	// Bind JSON request body
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request format")
		return
	}

	// Verify Google ID Token
	token, err := h.AuthService.VerifyGoogleIDToken(req.IDToken)
	if err != nil {
		utils.ErrorResponse(c, http.StatusUnauthorized, err.Error())
		return
	}

	// Respond with user details
	utils.SuccessResponse(c, http.StatusOK, "Login or Register successful", gin.H{
		"token": token,
	})
}

// store fcm token
func (h *AuthController) StoreFCMToken(c *gin.Context) {
	var req struct {
		FCMToken string `json:"fcm_token"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request format")
		return
	}

	userID := c.GetString("userID")
	if err := h.AuthService.StoreFCMToken(userID, req.FCMToken); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "FCM token stored successfully", nil)
}
