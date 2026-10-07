package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"audiobookshelf-go/internal/services"
)

// Username/Password are pointers so a field that is present-but-empty (passes
// through to the auth service, e.g. for passwordless root login) can be told
// apart from a field that is absent entirely (400) — mirrors passport-local's
// strategy.js, which only fails the request on a null (missing) field.
type loginRequest struct {
	Username *string `json:"username" form:"username"`
	Password *string `json:"password" form:"password"`
}

// Login handles POST /login. Like passport-local it answers 400 when a
// credential is missing and 401 when they are rejected, both as plain text.
func Login(auth *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req loginRequest
		// A body that fails to parse is treated like missing credentials.
		_ = c.ShouldBind(&req)
		if req.Username == nil || req.Password == nil {
			c.String(http.StatusBadRequest, http.StatusText(http.StatusBadRequest))
			return
		}

		returnTokens := c.GetHeader("x-return-tokens") == "true"
		result, err := auth.Login(c.Request.Context(), *req.Username, *req.Password, c.ClientIP(), c.Request.UserAgent(), returnTokens)
		if errors.Is(err, services.ErrInvalidCredentials) {
			c.String(http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}
		if err != nil {
			_ = c.Error(err)
			c.String(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
			return
		}

		if !returnTokens {
			setRefreshTokenCookie(c, result)
		}
		c.JSON(http.StatusOK, result.Response)
	}
}

// setRefreshTokenCookie writes the refresh_token cookie: httpOnly, SameSite=Lax,
// path "/", Secure only when the request arrived over HTTPS.
func setRefreshTokenCookie(c *gin.Context, result *services.AuthResult) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("refresh_token", result.RefreshToken, int(result.CookieMaxAge.Seconds()), "/", "", isRequestSecure(c), true)
}

// isRequestSecure reports whether the request used HTTPS, directly or via a
// proxy's x-forwarded-proto header (which may hold a list such as "http, https").
func isRequestSecure(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	for _, proto := range strings.Split(c.GetHeader("x-forwarded-proto"), ",") {
		if strings.EqualFold(strings.TrimSpace(proto), "https") {
			return true
		}
	}
	return false
}
