package handlers

import (
	"net/http"

	"audiobookshelf-go/internal/middleware"

	"github.com/gin-gonic/gin"
)

func Me(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	// A direct user object, as in Node, with only the current basic identity.
	c.JSON(http.StatusOK, gin.H{
		"id": user.ID, "username": user.Username, "type": user.Type, "isActive": user.IsActive,
	})
}
