package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type HealthHandler struct {
	db *gorm.DB
}

type healthData struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

func NewHealthHandler(db *gorm.DB) *HealthHandler {
	return &HealthHandler{db: db}
}

func (handler *HealthHandler) Show(c echo.Context) error {
	sqlDB, err := handler.db.DB()
	if err != nil {
		return NewError(
			http.StatusServiceUnavailable, "database_unavailable", "Database is unavailable",
			"The database handle could not be obtained", err,
		)
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return NewError(
			http.StatusServiceUnavailable, "database_unavailable", "Database is unavailable",
			"The database health check failed", err,
		)
	}
	return Success(c, http.StatusOK, healthData{Status: "ok", Database: "connected"})
}
