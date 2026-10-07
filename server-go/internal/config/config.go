package config

import "os"

type Config struct {
	Port   string
	DBPath string
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

	return Config{Port: port, DBPath: dbPath}
}
