package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterHealthCheck(e *gin.Engine, db *gorm.DB) {
	e.GET("/health", func(c *gin.Context) {
		if db != nil {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error"})
	})
}
