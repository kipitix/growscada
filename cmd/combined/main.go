package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
	"gitverse.ru/kipitix/gracedown"
	"gitverse.ru/kipitix/growscada/internal/application"
	"gitverse.ru/kipitix/growscada/internal/infrastructure/postgres/repositories"
	"gitverse.ru/kipitix/growscada/internal/interface/restapi"
	"gitverse.ru/kipitix/growscada/internal/interface/ui/root"

	_ "github.com/lib/pq"
)

const (
	databaseDSN = "postgres://growscada:growscada@localhost:5432/growscada?sslmode=disable"
)

func main() {
	// Настройка запуска сервера для раздачи клиентской части (PWA)
	app.Route("/", func() app.Composer {
		r := &root.Root{}
		r.SetMode(root.ModeOperation)
		return r
	})

	// Специальный вызов фреймворка go-app для запуска PWA
	app.RunWhenOnBrowser()

	// Создаём менеджер корректного завершения работы
	gracedownManager := gracedown.NewManager()

	// Сервер для раздачи клиента PWA
	pwaServer := &http.Server{
		Addr: ":8080",
		Handler: &app.Handler{
			Name:        "GrowSCADA",
			Description: "SCADA to Go",
		},
	}

	// Регистрируем обработчик завершения работы PWA сервера
	gracedownManager.RegisterInterface("PWA HTTP Server", 15*time.Second, func(ctx context.Context) error {
		return pwaServer.Shutdown(ctx)
	})

	// Запуск PWA сервера в отдельной рутине
	go func() {
		fmt.Println("🚀 PWA Server starting on :8080")
		if err := pwaServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("❌ PWA Server error: %v\n", err)
		}
	}()

	// API сервер
	// Создаём подключение в БД
	sqlDB, err := sql.Open("postgres", databaseDSN)
	if err != nil {
		fmt.Printf("❌ Database connection error: %v\n", err)
	}
	tarRepository := repositories.NewTagRepositoryPostgres(sqlDB)
	// Создаем сервисы
	tagService := application.NewTagService(tarRepository)
	// Создаем роутер
	apiRouter := restapi.NewRouter(tagService)

	// Запуск HTTP сервера для раздачи API
	apiServer := &http.Server{
		Addr:    ":9090",
		Handler: apiRouter.ServeMux(),
	}

	// Регистрируем обработчик завершения работы API сервера
	gracedownManager.RegisterInterface("API HTTP Server", 15*time.Second, func(ctx context.Context) error {
		return apiServer.Shutdown(ctx)
	})

	// Запуск API сервера в отдельной рутине
	go func() {
		fmt.Println("🚀 API Server starting on :9090")
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("❌ API Server error: %v\n", err)
		}
	}()

	// Ожидание сигнала завершения работы
	gracedownManager.WaitForSignal()

	fmt.Println("Server stopped")
}
