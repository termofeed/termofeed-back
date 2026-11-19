package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port            string
	PublicBaseURL   string
	AllowedOrigins  []string
	MaxParticipants int
	RetentionHours  int
	LogLevel        string
	TrustedProxy    bool
}

func Load() *Config {
	cfg := &Config{
		Port:            getEnv("PORT", "8080"),
		PublicBaseURL:   strings.TrimRight(getEnv("PUBLIC_BASE_URL", "http://localhost:5173"), "/"),
		AllowedOrigins:  parseOrigins(getEnv("ALLOWED_ORIGINS", "http://localhost:5173")),
		MaxParticipants: getEnvInt("MAX_PARTICIPANTS", 200),
		RetentionHours:  getEnvInt("RETENTION_HOURS", 24),
		LogLevel:        strings.ToLower(getEnv("LOG_LEVEL", "info")),
		TrustedProxy:    getEnvBool("TRUSTED_PROXY", false),
	}
	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil && i > 0 {
			return i
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}

func parseOrigins(val string) []string {
	var origins []string
	parts := strings.Split(val, ",")
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			origins = append(origins, strings.TrimRight(trimmed, "/"))
		}
	}
	return origins
}

func (c *Config) IsOriginAllowed(origin string) bool {
	if len(c.AllowedOrigins) == 0 {
		return true
	}
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	for _, allowed := range c.AllowedOrigins {
		if allowed == "*" || allowed == origin {
			return true
		}
	}
	return false
}
