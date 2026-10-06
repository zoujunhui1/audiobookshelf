package main

import (
	"log"

	"audiobookshelf-go/internal/config"
	"audiobookshelf-go/internal/router"
)

func main() {
	cfg := config.Load()
	r := router.New()

	log.Printf("listening on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
