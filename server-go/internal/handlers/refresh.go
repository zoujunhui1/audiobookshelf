package handlers

import (
	"errors"
	"log"
	"net/http"

	"audiobookshelf-go/internal/services"

	"github.com/gin-gonic/gin"
)

type RefreshHandler struct {
	auth *services.AuthService
}

func NewRefreshHandler(auth *services.AuthService) *RefreshHandler {
	return &RefreshHandler{auth: auth}
}

func (h *RefreshHandler) Refresh(c *gin.Context) {
	if h.auth == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Local authentication is not configured"})
		return
	}
	values := c.Request.Header.Values("x-refresh-token")
	headerMode := len(values) > 0
	value := ""
	if headerMode {
		value = values[0]
	} else if cookie, err := c.Request.Cookie("refresh_token"); err == nil {
		value = cookie.Value
	}
	if value == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "No refresh token provided"})
		return
	}
	result, err := h.auth.Refresh(c.Request.Context(), value)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidRefreshToken), errors.Is(err, services.ErrExpiredRefreshToken),
			errors.Is(err, services.ErrInvalidTokenType), errors.Is(err, services.ErrInactiveRefreshUser):
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		default:
			log.Printf("token refresh failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Refresh failed"})
		}
		return
	}
	// Node sets a cookie on successful refresh even when the header is used.
	setRefreshCookie(c, result)
	writeTokenResponse(c, result, headerMode)
}
