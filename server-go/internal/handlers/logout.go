package handlers

import (
	"log"
	"net/http"
	"time"

	"audiobookshelf-go/internal/services"

	"github.com/gin-gonic/gin"
)

type LogoutHandler struct {
	auth *services.AuthService
}

func NewLogoutHandler(auth *services.AuthService) *LogoutHandler {
	return &LogoutHandler{auth: auth}
}

func (h *LogoutHandler) Logout(c *gin.Context) {
	// Match the path used at login/refresh and clear the cookie even on failure.
	http.SetCookie(c.Writer, &http.Cookie{
		Name: "refresh_token", Value: "", Path: "/", MaxAge: -1,
		Expires: time.Unix(1, 0).UTC(), HttpOnly: true,
		Secure: requestIsSecure(c.Request), SameSite: http.SameSiteLaxMode,
	})
	if h.auth == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Local authentication is not configured"})
		return
	}
	// Unlike refresh, Node's logout prefers a nonempty cookie over the header.
	value := ""
	if cookie, err := c.Request.Cookie("refresh_token"); err == nil {
		value = cookie.Value
	}
	if value == "" {
		value = c.GetHeader("x-refresh-token")
	}
	if err := h.auth.Logout(c.Request.Context(), value, c.Query("allDevices") == "1"); err != nil {
		log.Printf("local logout failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Logout failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"redirect_url": nil})
}
