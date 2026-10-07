package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port          string
	DatabasePath  string
	JWTSecret     string
	AccessExpiry  time.Duration
	RefreshExpiry time.Duration
}

func Load() (Config, error) {
	port := os.Getenv("GO_SERVER_PORT")
	if port == "" {
		port = "4000"
	}
	accessExpiry, err := expiry("ACCESS_TOKEN_EXPIRY", time.Hour)
	if err != nil {
		return Config{}, err
	}
	refreshExpiry, err := expiry("REFRESH_TOKEN_EXPIRY", 30*24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Port:          port,
		DatabasePath:  os.Getenv("GO_DATABASE_PATH"),
		JWTSecret:     os.Getenv("JWT_SECRET_KEY"),
		AccessExpiry:  accessExpiry,
		RefreshExpiry: refreshExpiry,
	}, nil
}

func expiry(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	seconds, err := strconv.ParseInt(value, 10, 32)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer in seconds", name)
	}
	return time.Duration(seconds) * time.Second, nil
}
