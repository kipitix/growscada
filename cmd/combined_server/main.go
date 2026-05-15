package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/kipitix/gracedown"
	"github.com/kipitix/growscada/internal/application"
	"github.com/kipitix/growscada/internal/domain/event"
	"github.com/kipitix/growscada/internal/infrastructure/postgres/repositories"
	"github.com/kipitix/growscada/internal/interface/restapi"
	"github.com/kipitix/growscada/internal/interface/ui/root"
	"github.com/maxence-charriere/go-app/v10/pkg/app"

	_ "github.com/lib/pq"
)

const (
	databaseDSN  = "postgres://growscada:growscada@localhost:5432/growscada?sslmode=disable"
	apiServerURL = "http://localhost:9090"
)

type slogAdapter struct{}

func (slogAdapter) Println(v ...any) {
	slog.Info(fmt.Sprint(v...))
}

func main() {
	// Server setup for serving the client-side app (PWA)
	app.Route("/", func() app.Composer {
		r := root.NewRoot(apiServerURL)
		return r
	})

	// Required go-app framework call to initialize the PWA
	app.RunWhenOnBrowser()

	// Create a graceful shutdown manager
	gracedownManager := gracedown.NewManager(gracedown.WithLogger(slogAdapter{}))

	// INFRASTRUCTURE COMPONENTS

	// Create database connection
	sqlDB, err := sql.Open("postgres", databaseDSN)
	// Add hook to shutdown database connection
	gracedownManager.RegisterInfrastructure("Database", 15*time.Second, func(ctx context.Context) error {
		if sqlDB != nil {
			return sqlDB.Close()
		}
		return nil
	})
	// Check DB open error
	if err != nil {
		slog.Error("Database connection error", "error", err)
		emergencyExit(gracedownManager, gracedown.ExitIOErr)
	}
	// Check DB connection
	err = sqlDB.Ping()
	if err != nil {
		slog.Error("Database ping error", "error", err)
		emergencyExit(gracedownManager, gracedown.ExitIOErr)
	}

	// Create message broker connection
	// TODO
	// Add hook to shutdown message broker connection
	// Create event bus
	eventBus := event.NewEventBus()

	// INTERFACE COMPONENTS
	// API server
	tagRepository := repositories.NewTagRepositoryPostgres(sqlDB)
	indicatorTypeRepository := repositories.NewIndicatorTypeRepositoryPostgres(sqlDB)
	widgetTypeRepository := repositories.NewWidgetTypeRepositoryPostgres(sqlDB)
	// Create services
	tagService := application.NewTagService(tagRepository, eventBus)
	indicatorTypeService := application.NewIndicatorTypeService(indicatorTypeRepository, eventBus)
	widgetTypeService := application.NewWidgetTypeService(widgetTypeRepository, eventBus)
	// Create router
	apiRouter := restapi.NewRouter(tagService, indicatorTypeService, widgetTypeService)
	// Start HTTP server for API
	apiServer := &http.Server{
		Addr:    ":9090",
		Handler: apiRouter.ServeMux(),
	}

	// Register the API server shutdown handler
	gracedownManager.RegisterInterface("API HTTP Server", 15*time.Second, func(ctx context.Context) error {
		return apiServer.Shutdown(ctx)
	})

	// Start the API server in a separate goroutine
	go func() {
		slog.Info("API Server starting on :9090")
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("API Server error", "error", err)
		}
	}()

	// Server for serving the PWA client
	pwaServer := &http.Server{
		Addr: ":8080",
		Handler: &app.Handler{
			Name:        "GrowSCADA",
			Description: "SCADA to Go",
		},
	}
	// Register the PWA server shutdown handler
	gracedownManager.RegisterInterface("PWA HTTP Server", 15*time.Second, func(ctx context.Context) error {
		return pwaServer.Shutdown(ctx)
	})
	// Start the PWA server in a separate goroutine
	go func() {
		slog.Info("PWA Server starting on :8080")
		if err := pwaServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("PWA Server error", "error", err)
		}
	}()

	// Wait for shutdown signal
	err = gracedownManager.WaitForSignalAndShutdown()
	if err != nil {
		slog.Error("Error on graceful shutdown", "error", err)
		os.Exit(gracedown.ExitFailure)
	}

	slog.Info("Server stopped gracefully")

	os.Exit(gracedown.ExitSuccess)
}

// emergencyExit shuts down all registered components and exits with the given code.
func emergencyExit(manager *gracedown.Manager, exitCode int) {
	if shutdownErr := manager.EmergencyShutdown(); shutdownErr != nil {
		slog.Error("Emergency shutdown error", "error", shutdownErr)
	}
	os.Exit(exitCode)
}
