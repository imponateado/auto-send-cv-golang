package config

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// Config holds the application configuration.
type Config struct {
	Port string
	Env  string
}

// Load loads the configuration. It attempts to load environment variables
// from a local `.env` file first, without overriding existing shell variables.
func Load() *Config {
	// Attempt to load .env from the current working directory
	_ = LoadEnv(".env")

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

// LoadEnv reads a .env format file and sets variables in the environment
// if they are not already defined.
func LoadEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return ParseEnv(file)
}

// ParseEnv reads key-value pairs from an io.Reader and sets them in the environment.
func ParseEnv(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split line into key and value (limit to 2 parts)
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Strip surrounding single/double quotes from value
		if len(val) >= 2 {
			if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
				(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
				val = val[1 : len(val)-1]
			}
		}

		// Only set if the environment variable is not already defined (preserves parent env)
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
	return scanner.Err()
}
