package router

import (
	"github.com/gin-gonic/gin"

	"audiobookshelf-go/internal/handlers"
)

func New(auth *handlers.Auth) *gin.Engine {
	r := gin.Default()

	r.GET("/health", handlers.Health)
	r.POST("/login", auth.Login)

	return r
}
