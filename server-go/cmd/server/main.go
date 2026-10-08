package main

import (
	"database/sql"
	"log"

	_ "modernc.org/sqlite"

	"audiobookshelf-go/internal/config"
	"audiobookshelf-go/internal/repository"
	"audiobookshelf-go/internal/router"
	"audiobookshelf-go/internal/services"
)

func main() {
	cfg := config.Load()

	// sql.Open validates arguments only, it doesn't connect yet — a missing
	// DB file at this point is not fatal here, it will surface on first query.
	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		log.Fatalf("opening db at %s: %v", cfg.DBPath, err)
	}
	defer db.Close()

	users := repository.NewUserRepository(db)
	sessions := repository.NewSessionRepository(db)
	libraries := repository.NewLibraryRepository(db)
	authService := services.NewAuthService(users, sessions, libraries, cfg)

	r := router.New(authService)

	log.Printf("listening on :%s (db: %s)", cfg.Port, cfg.DBPath)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
