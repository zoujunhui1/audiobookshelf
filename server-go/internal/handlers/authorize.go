package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"audiobookshelf-go/internal/services"
)

// Authorize handles POST /authorize. It mirrors MiscController#authorize:
// the caller is already expected to be authenticated with a bearer access
// token (ApiRouter mounts /api behind the passport "jwt" strategy, which
// extracts the token from the Authorization header first, then the "token"
// query param — see Auth.js's ExtractJwt.fromExtractors), and the handler
// just returns that user's login response payload.
func Authorize(authService *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			token = c.Query("token")
		}

		result, err := authService.Authorize(c.Request.Context(), token)
		if err != nil {
			if errors.Is(err, services.ErrInvalidToken) || errors.Is(err, services.ErrUserInactive) {
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// bearerToken extracts the token from an "Authorization: Bearer <token>"
// header value, or returns "" if the header is absent or a different scheme.
// The scheme is matched case-insensitively, matching passport-jwt's
// fromAuthHeaderWithScheme (auth_scheme_lower === auth_params.scheme.toLowerCase()).
func bearerToken(header string) string {
	scheme, value, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return value
}
