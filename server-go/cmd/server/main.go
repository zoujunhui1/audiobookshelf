package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"audiobookshelf-go/internal/config"
	"audiobookshelf-go/internal/db"
	"audiobookshelf-go/internal/handlers"
	"audiobookshelf-go/internal/proxy"
	"audiobookshelf-go/internal/router"
	"audiobookshelf-go/internal/services"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	node, err := proxy.NewNode(proxy.NodeOptions{
		Port: cfg.NodePort, Dir: cfg.NodeDir,
		Executable: cfg.NodeExecutable, StartupTimeout: cfg.NodeStartupTimeout,
	})
	if err != nil {
		return err
	}
	defer node.Close()
	database, err := db.NewManager(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer database.Close()
	auth := services.NewAuth(database, node, cfg.JWTSecret, cfg.AccessTokenExpiry, cfg.RefreshTokenExpiry)
	r := router.New(node, handlers.Login(auth, cfg.AuthMaxAttempts, cfg.AuthWindow, cfg.AuthLimitMessage), cfg.RouterBasePath)
	server := &http.Server{Addr: ":" + cfg.Port, Handler: r, ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()

	log.Printf("listening on :%s", cfg.Port)
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		node.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
	}
	return nil
}
