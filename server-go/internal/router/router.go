package router

import (
	"github.com/gin-gonic/gin"

	"audiobookshelf-go/internal/handlers"
	"audiobookshelf-go/internal/services"
)

// New builds the gin engine. authService is threaded through now so the
// merge agent only has to add route registrations here, not wire up
// dependencies — see server-go/ai/workflow.md.
func New(authService *services.AuthService) *gin.Engine {
	r := gin.Default()

	r.GET("/health", handlers.Health)

	r.POST("/login", handlers.Login(authService))
	r.POST("/auth/refresh", handlers.Refresh(authService))
	r.POST("/logout", handlers.Logout(authService))
	// Node mounts this under ApiRouter's "/api" prefix (MiscController#authorize).
	r.POST("/api/authorize", handlers.Authorize(authService))

	return r
}
