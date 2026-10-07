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

	// login/refresh/logout/authorize routes land here during merge — see
	// server-go/ai/workflow.md

	r.GET("/health", handlers.Health)

	return r
}
