package config

import (
	"fmt"
	"os"
)

// Config holds runtime configuration, all sourced from environment variables.
type Config struct {
	HTTPAddr string
	APIKey   string
	DSN      string
}

// Load reads configuration from the environment. It returns an error if a
// required value (the API key) is missing so the process refuses to start
// unauthenticated.
func Load() (Config, error) {
	c := Config{
		HTTPAddr: env("HTTP_ADDR", ":8080"),
		APIKey:   os.Getenv("API_KEY"),
	}

	if c.APIKey == "" {
		return Config{}, fmt.Errorf("API_KEY is required")
	}

	// A full DSN wins if provided, otherwise assemble one from discrete parts.
	// Defaults mirror eBot-CSGO-Web/config/databases.yml (mysql ebotv3 on localhost).
	if dsn := os.Getenv("DB_DSN"); dsn != "" {
		c.DSN = dsn
	} else {
		host := env("DB_HOST", "127.0.0.1")
		port := env("DB_PORT", "3306")
		name := env("DB_NAME", "ebotv3")
		user := env("DB_USER", "root")
		pass := os.Getenv("DB_PASS")
		c.DSN = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&loc=Local",
			user, pass, host, port, name)
	}

	return c, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
