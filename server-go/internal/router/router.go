package router

import (
	"github.com/gin-gonic/gin"

	"audiobookshelf-go/internal/handlers"
	"audiobookshelf-go/internal/services"
)

func New(auth *services.AuthService) *gin.Engine {
	r := gin.Default()

	r.GET("/health", handlers.Health)
	r.POST("/login", handlers.NewLoginHandler(auth).Login)

	return r
}
