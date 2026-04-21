package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
	"github.com/kipitix/gracedown"
	"github.com/kipitix/growscada/internal/application"
	"github.com/kipitix/growscada/internal/infrastructure/postgres/repositories"
	"github.com/kipitix/growscada/internal/interface/restapi"
	"github.com/kipitix/growscada/internal/interface/ui/root"

	_ "github.com/lib/pq"
)

const (
	ExitSuccess = 0
	ExitFailure = 1
)

const (
	databaseDSN = "postgres://growscada:growscada@localhost:5432/growscada?sslmode=disable"
)

func main() {
	// Server setup for serving the client-side app (PWA)
	app.Route("/", func() app.Composer {
		r := &root.Root{}
		r.SetMode(root.ModeOperation)
		return r
	})

	// Required go-app framework call to initialize the PWA
	app.RunWhenOnBrowser()

	// Create a graceful shutdown manager
	gracedownManager := gracedown.NewManager()

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
		fmt.Println("🚀 PWA Server starting on :8080")
		if err := pwaServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("❌ PWA Server error: %v\n", err)
		}
	}()

	// API server
	// Create database connection
	sqlDB, err := sql.Open("postgres", databaseDSN)
	if err != nil {
		fmt.Printf("❌ Database connection error: %v\n", err)
		os.Exit(ExitFailure)
	}
	err = sqlDB.Ping()
	if err != nil {
		fmt.Printf("❌ Database ping error: %v\n", err)
		os.Exit(ExitFailure)
	}

	tagRepository := repositories.NewTagRepositoryPostgres(sqlDB)
	// Create services
	tagService := application.NewTagService(tagRepository)
	// Create router
	apiRouter := restapi.NewRouter(tagService)

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
		fmt.Println("🚀 API Server starting on :9090")
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("❌ API Server error: %v\n", err)
		}
	}()

	// Wait for shutdown signal
	err = gracedownManager.WaitForSignal()
	if err != nil {
		fmt.Printf("❌ Error on graceful shutdown: %v\n", err)
		os.Exit(ExitFailure)
	}

	fmt.Println("😎 Server stopped gracefully")

	os.Exit(ExitSuccess)
}
