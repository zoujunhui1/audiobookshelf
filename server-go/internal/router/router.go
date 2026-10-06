package router

import (
	"github.com/gin-gonic/gin"

	"audiobookshelf-go/internal/handlers"
)

func New() *gin.Engine {
	r := gin.Default()

	r.GET("/health", handlers.Health)

	return r
}
