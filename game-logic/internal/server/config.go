package server

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	ListenAddr       string
	RedisURL         string
	RedisKeyPrefix   string
	JWTSecret        []byte
	ServiceSecret    []byte
	LobbyCallbackURL string
	AllowedOrigins   []string
	TickInterval     time.Duration
	RoundPause       time.Duration
	StoreTimeout     time.Duration
	CallbackTimeout  time.Duration
}

func LoadConfig() (Config, error) {
	cfg := Config{
		ListenAddr:       envOr("GAME_LOGIC_ADDR", ":8081"),
		RedisKeyPrefix:   envOr("REDIS_KEY_PREFIX", "bololo:"),
		LobbyCallbackURL: envOr("LOBBY_CALLBACK_URL", "http://lobby-manager:8000"),
		TickInterval:     250 * time.Millisecond,
		RoundPause:       3 * time.Second,
		StoreTimeout:     200 * time.Millisecond,
		CallbackTimeout:  3 * time.Second,
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	serviceSecret := os.Getenv("SERVICE_SECRET")
	if jwtSecret == "" || serviceSecret == "" {
		return Config{}, errors.New("JWT_SECRET and SERVICE_SECRET are required")
	}
	cfg.JWTSecret = []byte(jwtSecret)
	cfg.ServiceSecret = []byte(serviceSecret)
	cfg.RedisURL = os.Getenv("REDIS_URL")
	if cfg.RedisURL == "" {
		return Config{}, errors.New("REDIS_URL is required")
	}
	if origins := strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS")); origins != "" {
		for _, origin := range strings.Split(origins, ",") {
			origin = strings.TrimSpace(origin)
			if origin != "" {
				cfg.AllowedOrigins = append(cfg.AllowedOrigins, origin)
			}
		}
	}
	var err error
	if cfg.TickInterval, err = durationEnv("TICK_INTERVAL", cfg.TickInterval); err != nil {
		return Config{}, err
	}
	if cfg.RoundPause, err = durationEnv("ROUND_PAUSE", cfg.RoundPause); err != nil {
		return Config{}, err
	}
	if cfg.StoreTimeout, err = durationEnv("REDIS_WRITE_TIMEOUT", cfg.StoreTimeout); err != nil {
		return Config{}, err
	}
	if cfg.CallbackTimeout, err = durationEnv("LOBBY_CALLBACK_TIMEOUT", cfg.CallbackTimeout); err != nil {
		return Config{}, err
	}
	if cfg.TickInterval <= 0 || cfg.RoundPause < 0 || cfg.StoreTimeout <= 0 || cfg.CallbackTimeout <= 0 {
		return Config{}, errors.New("service durations must be positive (ROUND_PAUSE may be zero)")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return duration, nil
}
