package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port               string
	NodePort           string
	NodeDir            string
	NodeExecutable     string
	NodeStartupTimeout time.Duration
	DatabasePath       string
	JWTSecret          string
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
	RouterBasePath     string
	AuthMaxAttempts    int
	AuthWindow         time.Duration
	AuthLimitMessage   string
}

func Load() (Config, error) {
	cfg := Config{
		Port:           envOr("GO_SERVER_PORT", "4000"),
		NodePort:       envOr("NODE_BACKEND_PORT", "3333"),
		NodeDir:        envOr("NODE_BACKEND_DIR", ".."),
		NodeExecutable: envOr("NODE_EXECUTABLE", "node"),
	}
	for _, port := range []string{cfg.Port, cfg.NodePort} {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return Config{}, fmt.Errorf("invalid server port %q", port)
		}
	}
	goPort, _ := strconv.Atoi(cfg.Port)
	nodePort, _ := strconv.Atoi(cfg.NodePort)
	if goPort == nodePort {
		return Config{}, fmt.Errorf("Go and Node must use different ports")
	}
	timeout, err := time.ParseDuration(envOr("NODE_STARTUP_TIMEOUT", "30s"))
	if err != nil || timeout <= 0 {
		return Config{}, fmt.Errorf("NODE_STARTUP_TIMEOUT must be a positive duration")
	}
	cfg.NodeStartupTimeout = timeout
	nodeDir, err := filepath.Abs(cfg.NodeDir)
	if err != nil {
		return Config{}, err
	}
	cfg.NodeDir = nodeDir
	configDir := envOr("CONFIG_PATH", "config")
	if !filepath.IsAbs(configDir) {
		configDir = filepath.Join(nodeDir, configDir)
	}
	cfg.DatabasePath = filepath.Join(configDir, "absdatabase.sqlite")
	cfg.JWTSecret = os.Getenv("JWT_SECRET_KEY")
	cfg.AccessTokenExpiry = time.Duration(positiveInt("ACCESS_TOKEN_EXPIRY", 3600)) * time.Second
	cfg.RefreshTokenExpiry = time.Duration(positiveInt("REFRESH_TOKEN_EXPIRY", 30*24*3600)) * time.Second
	basePath, exists := os.LookupEnv("ROUTER_BASE_PATH")
	if !exists {
		basePath = "/audiobookshelf"
	}
	cfg.RouterBasePath = strings.TrimRight(basePath, "/")
	if cfg.RouterBasePath != "" && !strings.HasPrefix(cfg.RouterBasePath, "/") {
		return Config{}, fmt.Errorf("ROUTER_BASE_PATH must start with /")
	}
	cfg.AuthMaxAttempts = positiveInt("RATE_LIMIT_AUTH_MAX", 40)
	if os.Getenv("RATE_LIMIT_AUTH_MAX") == "0" {
		cfg.AuthMaxAttempts = 0
	}
	cfg.AuthWindow = time.Duration(positiveInt("RATE_LIMIT_AUTH_WINDOW", 600000)) * time.Millisecond
	cfg.AuthLimitMessage = envOr("RATE_LIMIT_AUTH_MESSAGE", "Too many authentication requests")
	return cfg, nil
}

func positiveInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
