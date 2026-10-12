package router

import (
	"github.com/gin-gonic/gin"

	"audiobookshelf-go/internal/handlers"
	"audiobookshelf-go/internal/proxy"
)

func New(node *proxy.Node, login gin.HandlerFunc, basePath string) *gin.Engine {
	r := gin.Default()
	// Only explicit method/path registrations belong to Go. Preserve unmatched
	// requests, including trailing slashes, for the Node router.
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false

	r.GET("/health", handlers.Health)
	r.POST("/login", login)
	if basePath != "" {
		r.POST(basePath+"/login", login)
	}
	r.NoRoute(gin.WrapH(proxy.Forward(node)))

	return r
}
