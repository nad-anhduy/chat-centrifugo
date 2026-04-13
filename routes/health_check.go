package routes

import (
	"context"
	"net/http"

	"github.com/centrifugal/gocent/v3"
	"github.com/gin-gonic/gin"
	"github.com/gocql/gocql"
	"gorm.io/gorm"
)

func RegisterHealthCheck(e *gin.Engine, db *gorm.DB, scyllaSession *gocql.Session, centrifugoClient *gocent.Client) {
	e.GET("/health", func(c *gin.Context) {
		status := make(map[string]string)
		overallStatus := "ok"

		// Check Postgres
		if err := db.Raw("SELECT 1").Error; err != nil {
			status["postgres"] = "error: " + err.Error()
			overallStatus = "error"
		} else {
			status["postgres"] = "ok"
		}

		// Check ScyllaDB
		if err := scyllaSession.Query("SELECT now() FROM system.local").Exec(); err != nil {
			status["scylladb"] = "error: " + err.Error()
			overallStatus = "error"
		} else {
			status["scylladb"] = "ok"
		}

		// Check Centrifugo
		if _, err := centrifugoClient.Info(context.Background()); err != nil {
			status["centrifugo"] = "error: " + err.Error()
			overallStatus = "error"
		} else {
			status["centrifugo"] = "ok"
		}

		status["status"] = overallStatus
		if overallStatus == "ok" {
			c.JSON(http.StatusOK, status)
		} else {
			c.JSON(http.StatusServiceUnavailable, status)
		}
	})
}
