package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"audiobookshelf-go/internal/config"
	"audiobookshelf-go/internal/database"
	"audiobookshelf-go/internal/repository"
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
	var auth *services.AuthService
	if cfg.DatabasePath != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		db, err := database.OpenSQLite(ctx, cfg.DatabasePath)
		if err != nil {
			cancel()
			return err
		}
		defer db.Close()
		secret := cfg.JWTSecret
		if secret == "" {
			secret, err = repository.LoadJWTSecret(ctx, db)
		}
		cancel()
		if err != nil {
			return err
		}
		auth, err = services.NewAuthService(
			repository.NewUserRepository(db), repository.NewSessionRepository(db),
			secret, cfg.AccessExpiry, cfg.RefreshExpiry,
		)
		if err != nil {
			return err
		}
		log.Printf("opened existing SQLite database: %s", cfg.DatabasePath)
	}

	r := router.New(auth)

	log.Printf("listening on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		return fmt.Errorf("serve HTTP: %w", err)
	}
	return nil
}
