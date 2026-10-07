package middleware

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"audiobookshelf-go/internal/repository"
	"audiobookshelf-go/internal/services"

	"github.com/gin-gonic/gin"
)

const userContextKey = "authenticatedUser"

func Authenticate(auth *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if auth == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "Local authentication is not configured"})
			return
		}
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			unauthorized(c)
			return
		}
		user, err := auth.AuthenticateAccessToken(c.Request.Context(), parts[1])
		if errors.Is(err, services.ErrInvalidAccessToken) {
			unauthorized(c)
			return
		}
		if err != nil {
			log.Printf("access-token authentication failed: %v", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Authentication failed"})
			return
		}
		c.Set(userContextKey, user)
		c.Next()
	}
}

func unauthorized(c *gin.Context) {
	c.Header("WWW-Authenticate", "Bearer")
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
}

func CurrentUser(c *gin.Context) *repository.User {
	value, _ := c.Get(userContextKey)
	user, _ := value.(*repository.User)
	return user
}
