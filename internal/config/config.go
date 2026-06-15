package config

import (
	"os"
)

// Config holds the application configuration.
type Config struct {
	Port string
	Env  string
}

// Load loads the configuration from environment variables with sensible defaults.
func Load() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	return &Config{
		Port: port,
		Env:  env,
	}
}
