package handlers

import (
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"audiobookshelf-go/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type LoginHandler struct {
	auth *services.AuthService
}

func NewLoginHandler(auth *services.AuthService) *LoginHandler {
	return &LoginHandler{auth: auth}
}

func (h *LoginHandler) Login(c *gin.Context) {
	if h.auth == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Local authentication is not configured"})
		return
	}
	// Pointers distinguish an explicit empty root password from a missing field.
	var request struct {
		Username *string `json:"username" form:"username"`
		Password *string `json:"password" form:"password"`
	}
	var err error
	switch c.ContentType() {
	case "application/json":
		err = c.ShouldBindJSON(&request)
	case "application/x-www-form-urlencoded":
		err = c.ShouldBindWith(&request, binding.FormPost)
	default:
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "Use JSON or URL-encoded credentials"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid login request"})
		return
	}
	if request.Username == nil || *request.Username == "" || request.Password == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing credentials"})
		return
	}
	ipAddress, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		ipAddress = c.Request.RemoteAddr
	}
	result, err := h.auth.Login(c.Request.Context(), *request.Username, *request.Password,
		ipAddress, c.GetHeader("User-Agent"))
	if errors.Is(err, services.ErrInvalidCredentials) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}
	if err != nil {
		log.Printf("local login failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Login failed"})
		return
	}
	var refreshToken *string
	if c.GetHeader("x-return-tokens") == "true" {
		refreshToken = &result.RefreshToken
	} else {
		http.SetCookie(c.Writer, &http.Cookie{
			Name: "refresh_token", Value: result.RefreshToken, Path: "/",
			HttpOnly: true, Secure: requestIsSecure(c.Request), SameSite: http.SameSiteLaxMode,
			MaxAge:  int(result.RefreshExpiresAt.Sub(result.IssuedAt) / time.Second),
			Expires: result.RefreshExpiresAt,
		})
	}
	// Deliberate reduced projection: never serialize the repository's password hash.
	c.JSON(http.StatusOK, gin.H{"user": gin.H{
		"id": result.User.ID, "username": result.User.Username,
		"type": result.User.Type, "isActive": result.User.IsActive,
		"accessToken": result.AccessToken, "refreshToken": refreshToken,
	}})
}

func requestIsSecure(request *http.Request) bool {
	if request.TLS != nil {
		return true
	}
	for _, protocol := range strings.Split(request.Header.Get("x-forwarded-proto"), ",") {
		if strings.EqualFold(strings.TrimSpace(protocol), "https") {
			return true
		}
	}
	return false
}
