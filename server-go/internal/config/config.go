package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	Port      string
	DBPath    string
	JWTSecret string
}

func Load() Config {
	port := os.Getenv("GO_SERVER_PORT")
	if port == "" {
		port = "4000"
	}

	dbPath := os.Getenv("ABS_DB_PATH")
	if dbPath == "" {
		configPath := os.Getenv("CONFIG_PATH")
		if configPath == "" {
			configPath = "config"
		}
		dbPath = filepath.Join(configPath, "absdatabase.sqlite")
	}

	jwtSecret := os.Getenv("JWT_SECRET_KEY")
	if jwtSecret == "" {
		jwtSecret = "audiobookshelf-dev-secret"
	}

	return Config{
		Port:      port,
		DBPath:    dbPath,
		JWTSecret: jwtSecret,
	}
}
