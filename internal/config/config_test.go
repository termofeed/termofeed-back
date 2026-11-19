package config_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/termofeed/termofeed-back/internal/config"
)

func TestConfig_Defaults(t *testing.T) {
	os.Clearenv()

	cfg := config.Load()
	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, "http://localhost:5173", cfg.PublicBaseURL)
	assert.Equal(t, []string{"http://localhost:5173"}, cfg.AllowedOrigins)
	assert.Equal(t, 200, cfg.MaxParticipants)
	assert.Equal(t, 24, cfg.RetentionHours)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.False(t, cfg.TrustedProxy)
}

func TestConfig_CustomEnv(t *testing.T) {
	os.Clearenv()
	os.Setenv("PORT", "9090")
	os.Setenv("PUBLIC_BASE_URL", "https://example.com/")
	os.Setenv("ALLOWED_ORIGINS", "https://example.com, https://app.example.com/")
	os.Setenv("MAX_PARTICIPANTS", "150")
	os.Setenv("RETENTION_HOURS", "48")
	os.Setenv("LOG_LEVEL", "DEBUG")
	os.Setenv("TRUSTED_PROXY", "true")

	cfg := config.Load()
	assert.Equal(t, "9090", cfg.Port)
	assert.Equal(t, "https://example.com", cfg.PublicBaseURL)
	assert.Equal(t, []string{"https://example.com", "https://app.example.com"}, cfg.AllowedOrigins)
	assert.Equal(t, 150, cfg.MaxParticipants)
	assert.Equal(t, 48, cfg.RetentionHours)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.True(t, cfg.TrustedProxy)

	assert.True(t, cfg.IsOriginAllowed("https://example.com"))
	assert.True(t, cfg.IsOriginAllowed("https://example.com/"))
	assert.True(t, cfg.IsOriginAllowed("https://app.example.com"))
	assert.False(t, cfg.IsOriginAllowed("https://other.com"))
}
