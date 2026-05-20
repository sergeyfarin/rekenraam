package config

import "os"

type Config struct {
	HTTPAddr    string
	DatabaseURL string
}

func Load() Config {
	return Config{
		HTTPAddr:    env("HTTP_ADDR", ":18888"),
		DatabaseURL: env("DATABASE_URL", "file:var/dev.sqlite"),
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
