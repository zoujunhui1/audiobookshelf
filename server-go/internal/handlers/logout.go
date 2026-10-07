package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"audiobookshelf-go/internal/services"
)

// Logout handles POST /logout. It mirrors Auth.js's /logout route: the
// refresh token is read from the refresh_token cookie, falling back to the
// X-Refresh-Token header; ?allDevices=1 invalidates every session for the
// user instead of just the current one; the refresh_token cookie is always
// cleared.
func Logout(authService *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Cookie takes priority over the header, same as
		// `req.cookies.refresh_token || req.headers['x-refresh-token']`.
		refreshToken, _ := c.Cookie("refresh_token") // no cookie is a normal, expected case
		if refreshToken == "" {
			refreshToken = c.GetHeader("x-refresh-token")
		}
		allDevices := c.Query("allDevices") == "1"

		c.SetCookie("refresh_token", "", -1, "/", "", false, true)

		result, err := authService.Logout(refreshToken, allDevices)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		c.JSON(http.StatusOK, result)
	}
}
