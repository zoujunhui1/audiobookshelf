package config

import "os"

type Config struct {
	Port         string
	DatabasePath string
}

func Load() Config {
	port := os.Getenv("GO_SERVER_PORT")
	if port == "" {
		port = "4000"
	}
	return Config{
		Port:         port,
		DatabasePath: os.Getenv("GO_DATABASE_PATH"),
	}
}
