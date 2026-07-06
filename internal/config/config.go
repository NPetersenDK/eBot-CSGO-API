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

	// A full DSN wins if provided, otherwise assemble one from the same MYSQL_*
	// variables the eBot docker-compose stack already uses (see eBot-docker/.env).
	if dsn := os.Getenv("DB_DSN"); dsn != "" {
		c.DSN = dsn
	} else {
		host := env("MYSQL_HOST", "mysqldb")
		port := env("MYSQL_PORT", "3306")
		name := env("MYSQL_DATABASE", "ebotv3")
		user := env("MYSQL_USER", "ebotv3")
		pass := os.Getenv("MYSQL_PASSWORD")
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
