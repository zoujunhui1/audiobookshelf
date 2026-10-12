package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"audiobookshelf-go/internal/db"
	"audiobookshelf-go/internal/services"
)

type loginWindow struct {
	start time.Time
	count int
}

// Login owns HTTP parsing, cookies and a per-client login rate limiter.
func Login(auth *services.Auth, maxAttempts int, window time.Duration, limitMessage string) gin.HandlerFunc {
	var mu sync.Mutex
	attempts := make(map[string]loginWindow)
	return func(c *gin.Context) {
		ip := c.RemoteIP()
		if maxAttempts > 0 {
			now := time.Now()
			mu.Lock()
			for key, value := range attempts {
				if now.Sub(value.start) >= window {
					delete(attempts, key)
				}
			}
			value := attempts[ip]
			if value.start.IsZero() {
				value.start = now
			}
			value.count++
			attempts[ip] = value
			mu.Unlock()
			if value.count > maxAttempts {
				c.JSON(http.StatusTooManyRequests, gin.H{"error": limitMessage})
				return
			}
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20)
		var credentials struct {
			Username *string `json:"username" form:"username"`
			Password *string `json:"password" form:"password"`
		}
		if c.ShouldBind(&credentials) != nil || credentials.Username == nil || credentials.Password == nil {
			c.String(http.StatusBadRequest, "Bad Request")
			return
		}
		returnTokens := c.GetHeader("x-return-tokens") == "true"
		result, err := auth.Login(c.Request.Context(), *credentials.Username, *credentials.Password, ip, c.GetHeader("User-Agent"), returnTokens)
		if errors.Is(err, db.ErrInvalidCredentials) {
			c.String(http.StatusUnauthorized, "Unauthorized")
			return
		}
		if err != nil {
			log.Printf("Go login failed: %v", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Login service unavailable"})
			return
		}
		if !returnTokens {
			secure := c.Request.TLS != nil
			// Trust protocol metadata only from the local upstream proxy.
			if ip == "127.0.0.1" || ip == "::1" {
				for _, protocol := range strings.Split(c.GetHeader("X-Forwarded-Proto"), ",") {
					if strings.EqualFold(strings.TrimSpace(protocol), "https") {
						secure = true
					}
				}
			}
			http.SetCookie(c.Writer, &http.Cookie{Name: "refresh_token", Value: result.RefreshToken, Path: "/",
				HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
				MaxAge: int(time.Until(result.RefreshExpiresAt).Seconds()) + 1, Expires: result.RefreshExpiresAt})
		}
		c.JSON(http.StatusOK, result.Payload)
	}
}
