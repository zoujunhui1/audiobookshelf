package config

import "os"

type Config struct {
	Port string
}

func Load() Config {
	port := os.Getenv("GO_SERVER_PORT")
	if port == "" {
		port = "4000"
	}
	return Config{Port: port}
}
