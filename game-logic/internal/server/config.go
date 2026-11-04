package server

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/chameleon"
)

type Config struct {
	ListenAddr     string
	RedisURL       string
	RedisKeyPrefix string
	JWTSecret      []byte
	ServiceSecret  []byte
	RabbitMQURL    string
	AllowedOrigins []string
	TickInterval   time.Duration
	RoundPause     time.Duration
	StoreTimeout   time.Duration
	PublishTimeout time.Duration
	// Chameleon parametriza el repartidor del modo camaleón (cero = valores por defecto).
	Chameleon chameleon.Config
}

func LoadConfig() (Config, error) {
	cfg := Config{
		ListenAddr:     envOr("GAME_LOGIC_ADDR", ":8081"),
		RedisKeyPrefix: envOr("REDIS_KEY_PREFIX", "bololo:"),
		TickInterval:   250 * time.Millisecond,
		RoundPause:     3 * time.Second,
		StoreTimeout:   200 * time.Millisecond,
		PublishTimeout: 5 * time.Second,
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
	cfg.RabbitMQURL = os.Getenv("RABBITMQ_URL")
	if cfg.RabbitMQURL == "" {
		return Config{}, errors.New("RABBITMQ_URL is required")
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
	if cfg.PublishTimeout, err = durationEnv("RESULT_PUBLISH_TIMEOUT", cfg.PublishTimeout); err != nil {
		return Config{}, err
	}
	if cfg.TickInterval <= 0 || cfg.RoundPause < 0 || cfg.StoreTimeout <= 0 || cfg.PublishTimeout <= 0 {
		return Config{}, errors.New("service durations must be positive (ROUND_PAUSE may be zero)")
	}
	// Modo camaleón: duración de la partida (por defecto 3m), útil para demos cortas.
	if cfg.Chameleon.GameDuration, err = durationEnv("CHAMELEON_GAME_DURATION", 0); err != nil {
		return Config{}, err
	}
	if cfg.Chameleon.GameDuration < 0 {
		return Config{}, errors.New("CHAMELEON_GAME_DURATION must be positive")
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
