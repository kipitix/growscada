package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/kipitix/gracedown"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
	"github.com/kipitix/growscada/internal/server/interface/ui/root"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	_ "github.com/lib/pq"
)

const (
	databaseDSN  = "postgres://growscada:growscada@localhost:5432/growscada?sslmode=disable"
	apiServerURL = "http://localhost:9090"
)

// serverArgs is the server's command-line/environment configuration.
type serverArgs struct {
	MaxSSEClients int `arg:"env:MAX_SSE_CLIENTS" default:"100" help:"maximum number of concurrent SSE event-stream connections"`
}

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

	var cliArgs serverArgs
	arg.MustParse(&cliArgs)

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
	// Create event bus: the SSE stream's source of events
	eventBus := eventbus.NewEventBus()
	// Create the outbox dispatcher: it delivers the events of committed
	// changes to the event bus and enqueues the policies' jobs (ADR 0008).
	// Repositories wake it after each commit that wrote events
	dispatcher := outbox.NewDispatcher(sqlDB, eventBus.Publish)
	notifyDispatcher := repositories.NotifyOnCommit(dispatcher.Notify)
	tagRepository := repositories.NewTagRepositoryPostgres(sqlDB, notifyDispatcher)
	widgetTypeRepository := repositories.NewWidgetTypeRepositoryPostgres(sqlDB, notifyDispatcher)
	sceneRepository := repositories.NewSceneRepositoryPostgres(sqlDB, notifyDispatcher)
	// Registry of the policies: changes spanning aggregates, run as jobs
	// apart from event delivery (ADR 0008)
	policyRunner := outbox.NewPolicyRunner(sqlDB, []application.Policy{
		application.NewWidgetReconciliationPolicy(widgetTypeRepository, sceneRepository, tagRepository),
		application.NewUnbindDeletedTagPolicy(sceneRepository, tagRepository),
	})
	dispatcher.SetPolicyRunner(policyRunner)
	dispatcherCtx, stopDispatcher := context.WithCancel(context.Background())
	dispatcherDone := make(chan struct{})
	go func() {
		dispatcher.Run(dispatcherCtx)
		close(dispatcherDone)
	}()
	// stopOutbox stops the dispatcher and delivers what is left in the outbox
	stopOutbox := func(ctx context.Context) {
		stopDispatcher()
		select {
		case <-dispatcherDone:
		case <-ctx.Done():
			slog.Error("Outbox dispatcher did not stop on shutdown", "error", ctx.Err())
			return
		}
		if _, err := dispatcher.DrainOnce(ctx); err != nil {
			slog.Error("Outbox not drained on shutdown", "error", err)
		}
	}
	// Run the policy jobs. On shutdown the job running is stopped and runs
	// again after the next start. It stops with the interface layer, so
	// before the database closes in the infrastructure layer
	policyCtx, stopPolicies := context.WithCancel(context.Background())
	policiesDone := make(chan struct{})
	go func() {
		policyRunner.Run(policyCtx)
		close(policiesDone)
	}()
	gracedownManager.RegisterInterface("Policy Runner", 15*time.Second, func(ctx context.Context) error {
		stopPolicies()
		select {
		case <-policiesDone:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	// INTERFACE COMPONENTS
	// API server
	// Create services
	tagService := application.NewTagService(tagRepository)
	widgetTypeService := application.NewWidgetTypeService(widgetTypeRepository)
	sceneService := application.NewSceneService(sceneRepository, widgetTypeRepository, tagRepository)
	// Create router
	apiRouter := restapi.NewRouter(tagService, widgetTypeService, sceneService, eventBus, cliArgs.MaxSSEClients)
	// Register the event hub shutdown handler. The outbox is drained first,
	// while SSE clients are still connected, so what is left in it reaches
	// them; a change committed after the drain (the API server stops
	// concurrently) stays in the outbox until the next start. The drain gets
	// outboxDrainTimeout of the API server's 15 s, so the hub is closed in
	// time even if the database hangs. Then the hub ends every open SSE
	// stream: the API server's Shutdown, running concurrently, waits for
	// those handlers and is released here. The database closes later, in
	// the infrastructure layer.
	const outboxDrainTimeout = 5 * time.Second
	gracedownManager.RegisterInterface("Event Hub", 15*time.Second, func(ctx context.Context) error {
		drainCtx, cancel := context.WithTimeout(ctx, outboxDrainTimeout)
		defer cancel()
		stopOutbox(drainCtx)
		apiRouter.Close()
		return nil
	})
	// Start HTTP server for API
	apiServer := &http.Server{
		Addr:    ":9090",
		Handler: apiRouter.ServeMux(),
	}

	// Register the API server shutdown handler. Connections still open when
	// Shutdown gives up are closed, so no handler outlives the shutdown
	gracedownManager.RegisterInterface("API HTTP Server", 15*time.Second, func(ctx context.Context) error {
		err := apiServer.Shutdown(ctx)
		if err != nil {
			apiServer.Close() //nolint:errcheck
		}
		return err
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
