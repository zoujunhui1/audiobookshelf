package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"audiobookshelf-go/internal/config"
	"audiobookshelf-go/internal/database"
	"audiobookshelf-go/internal/router"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := config.Load()
	if cfg.DatabasePath != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		db, err := database.OpenSQLite(ctx, cfg.DatabasePath)
		cancel()
		if err != nil {
			return err
		}
		defer db.Close()
		log.Printf("opened existing SQLite database: %s", cfg.DatabasePath)
	}

	r := router.New()

	log.Printf("listening on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		return fmt.Errorf("serve HTTP: %w", err)
	}
	return nil
}
