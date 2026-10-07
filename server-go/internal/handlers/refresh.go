package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"audiobookshelf-go/internal/services"
)

// Refresh handles POST /auth/refresh. The refresh token comes from the
// refresh_token cookie, or from the x-refresh-token header, which takes
// precedence and also makes the response body include the new refresh token.
func Refresh(auth *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		refreshToken, _ := c.Cookie("refresh_token")
		fromHeader := false
		if h := c.GetHeader("x-refresh-token"); h != "" {
			refreshToken = h
			fromHeader = true
		}
		if refreshToken == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "No refresh token provided"})
			return
		}

		result, err := auth.Refresh(c.Request.Context(), refreshToken, fromHeader)
		if err != nil {
			if msg, ok := refreshErrorMessage(err); ok {
				c.JSON(http.StatusUnauthorized, gin.H{"error": msg})
				return
			}
			_ = c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}

		setRefreshTokenCookie(c, result)
		c.JSON(http.StatusOK, result.Response)
	}
}

// refreshErrorMessage maps a service error to the message Node sends with its 401.
func refreshErrorMessage(err error) (string, bool) {
	switch {
	case errors.Is(err, services.ErrInvalidTokenType):
		return "Invalid token type", true
	case errors.Is(err, services.ErrInvalidRefreshToken):
		return "Invalid refresh token", true
	case errors.Is(err, services.ErrRefreshTokenExpired):
		return "Refresh token expired", true
	case errors.Is(err, services.ErrUserInactive):
		return "User not found or inactive", true
	}
	return "", false
}
