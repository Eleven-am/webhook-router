package config

import (
	"os"
)

type Config struct {
	Port         string
	DatabasePath string
	RabbitMQURL  string
	DefaultQueue string
	LogLevel     string
}

func Load() *Config {
	return &Config{
		Port:         getEnv("PORT", "8080"),
		DatabasePath: getEnv("DATABASE_PATH", "./webhook_router.db"),
		RabbitMQURL:  getEnv("RABBITMQ_URL", ""),
		DefaultQueue: getEnv("DEFAULT_QUEUE", "webhooks"),
		LogLevel:     getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
