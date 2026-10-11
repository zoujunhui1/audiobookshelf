package main

import (
	"database/sql"
	"log"

	_ "modernc.org/sqlite"

	"audiobookshelf-go/internal/config"
	"audiobookshelf-go/internal/handlers"
	"audiobookshelf-go/internal/repository"
	"audiobookshelf-go/internal/router"
)

func main() {
	cfg := config.Load()

	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("ping sqlite %s: %v", cfg.DBPath, err)
	}

	auth := &handlers.Auth{
		Users:     repository.NewUserRepo(db),
		JWTSecret: []byte(cfg.JWTSecret),
	}
	r := router.New(auth)

	log.Printf("listening on :%s (db %s)", cfg.Port, cfg.DBPath)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
