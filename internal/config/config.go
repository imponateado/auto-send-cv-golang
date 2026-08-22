package config

import (
	"bufio"
	"io"
	"os"
	"strings"
)

type Config struct {
	Port string
	Env  string
}

// Load lê o `.env` local (sem sobrescrever variáveis já definidas no shell) e
// monta a Config a partir de PORT/APP_ENV, com defaults "8080"/"development".
// Sempre retorna uma *Config não-nula.
func Load() *Config {
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

// LoadEnv abre o arquivo em path e delega a ParseEnv. Retorna erro se o arquivo
// não puder ser aberto ou se o parse falhar.
func LoadEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return ParseEnv(file)
}

// ParseEnv reads KEY=VALUE lines from r and calls os.Setenv for each key not
// already defined in the environment. Returns an error only if scanning r
// fails.
func ParseEnv(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		if len(val) >= 2 {
			if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
				(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
				val = val[1 : len(val)-1]
			}
		}

		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
	return scanner.Err()
}
