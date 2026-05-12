package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	GitLabBaseURL     string
	GitLabAccessToken string
	GitLabProjectPath string
	MySQLHost         string
	MySQLPort         int
	MySQLUser         string
	MySQLPassword     string
	MySQLDatabase     string
	HTTPAddr          string
}

func Load() (*Config, error) {
	_ = godotenv.Load(".env", "../.env")

	cfg := &Config{
		GitLabBaseURL:     strings.TrimRight(getEnv("GITLAB_BASE_URL", "https://gitlab.zetpy.com"), "/"),
		GitLabAccessToken: strings.TrimSpace(getEnv("GITLAB_ACCESS_TOKEN", "")),
		GitLabProjectPath: strings.TrimSpace(getEnv("GITLAB_PROJECT_PATH", "zetpy/zetpy-core")),
		MySQLHost:         getEnv("MYSQL_HOST", "127.0.0.1"),
		MySQLUser:         getEnv("MYSQL_USER", "zetpy"),
		MySQLPassword:     os.Getenv("MYSQL_PASSWORD"),
		MySQLDatabase:     getEnv("MYSQL_DATABASE", "gitlab_issues"),
		HTTPAddr:          getEnv("HTTP_ADDR", ":8080"),
	}

	port, err := strconv.Atoi(getEnv("MYSQL_PORT", "3306"))
	if err != nil {
		return nil, fmt.Errorf("MYSQL_PORT: %w", err)
	}
	cfg.MySQLPort = port

	if cfg.GitLabAccessToken == "" {
		return nil, fmt.Errorf("GITLAB_ACCESS_TOKEN is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
