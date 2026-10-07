package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port   string
	DBPath string

	// JWT — must match the Node.js server's values exactly (see .env.example),
	// otherwise tokens issued by one side won't validate on the other.
	JWTSecret          string
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
	RefreshGracePeriod time.Duration
}

func Load() Config {
	port := os.Getenv("GO_SERVER_PORT")
	if port == "" {
		port = "4000"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		// Same file the Node.js server opens at <CONFIG_PATH>/absdatabase.sqlite,
		// with CONFIG_PATH defaulting to "config" at the repo root (see index.js).
		dbPath = "../config/absdatabase.sqlite"
	}

	return Config{
		Port:               port,
		DBPath:             dbPath,
		JWTSecret:          os.Getenv("JWT_SECRET_KEY"),
		AccessTokenExpiry:  envSeconds("ACCESS_TOKEN_EXPIRY", 3600),
		RefreshTokenExpiry: envSeconds("REFRESH_TOKEN_EXPIRY", 30*24*3600),
		RefreshGracePeriod: envSeconds("REFRESH_TOKEN_GRACE_PERIOD", 600),
	}
}

func envSeconds(key string, defaultSeconds int) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	return time.Duration(defaultSeconds) * time.Second
}
