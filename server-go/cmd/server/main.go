package main

import (
	"log"

	"audiobookshelf-go/internal/config"
	"audiobookshelf-go/internal/router"
)

func main() {
	cfg := config.Load()
	r := router.New()

	log.Printf("listening on :%s (db: %s)", cfg.Port, cfg.DBPath)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
