package handlers

import (
	"net/http"
	"strings"
	"time"

	"audiobookshelf-go/internal/services"

	"github.com/gin-gonic/gin"
)

func setRefreshCookie(c *gin.Context, result *services.LoginResult) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: "refresh_token", Value: result.RefreshToken, Path: "/",
		HttpOnly: true, Secure: requestIsSecure(c.Request), SameSite: http.SameSiteLaxMode,
		MaxAge:  int(result.RefreshExpiresAt.Sub(result.IssuedAt) / time.Second),
		Expires: result.RefreshExpiresAt,
	})
}

func writeTokenResponse(c *gin.Context, result *services.LoginResult, returnRefreshToken bool) {
	var refreshToken *string
	if returnRefreshToken {
		refreshToken = &result.RefreshToken
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
