package config

import "os"

type Config struct {
	AppEnv      string
	HTTPAddr    string
	DatabaseURL string
}

func Load() Config {
	return Config{
		AppEnv:      env("APP_ENV", "production"),
		HTTPAddr:    env("HTTP_ADDR", ":16888"),
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
