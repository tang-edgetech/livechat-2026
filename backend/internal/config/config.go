package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config holds every environment-driven setting. Nothing here is ever
// hardcoded elsewhere in the codebase — see overview.md §5/§9 "Port &
// domain flexibility": the same build must run unchanged on XAMPP
// localhost and on a live server, only these values change.
type Config struct {
	AppPort     string
	WSPort      string
	BaseURL     string
	DBHost      string
	DBPort      string
	DBName      string
	DBUser      string
	DBPassword  string
	RedisAddr   string
	RedisPassword string
	UploadsPath string
	MigrationsPath string
	FrontendOrigin string
}

// Load reads .env (if present) then falls back to process env vars / defaults.
// Missing .env is not an error — a fresh checkout with no wizard run yet
// should still be able to boot far enough to serve the Setup Wizard.
func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		AppPort:       getEnv("APP_PORT", "8080"),
		WSPort:        getEnv("WS_PORT", "8081"),
		BaseURL:       getEnv("BASE_URL", "http://localhost:8080"),
		DBHost:        getEnv("DB_HOST", "127.0.0.1"),
		DBPort:        getEnv("DB_PORT", "3306"),
		DBName:        getEnv("DB_NAME", "livechat"),
		DBUser:        getEnv("DB_USER", "root"),
		DBPassword:    getEnv("DB_PASSWORD", ""),
		RedisAddr:     getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		UploadsPath:   getEnv("UPLOADS_PATH", "./uploads"),
		MigrationsPath: getEnv("MIGRATIONS_PATH", "./migrations"),
		FrontendOrigin: getEnv("FRONTEND_ORIGIN", "http://localhost:3000"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
