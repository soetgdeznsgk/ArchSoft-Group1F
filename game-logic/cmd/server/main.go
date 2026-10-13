package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/server"
	redisstore "github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/store/redis"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "check the local Game Logic health endpoint and exit")
	flag.Parse()
	if *healthcheck {
		os.Exit(checkHealth())
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("Game Logic server stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := server.LoadConfig()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	client, err := redisstore.NewClient(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("create Redis client: %w", err)
	}
	store := redisstore.New(client, cfg.RedisKeyPrefix)
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error("close Redis client", "error", err)
		}
	}()

	startupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		err = store.Ping(startupCtx)
		if err == nil {
			break
		}
		if startupCtx.Err() != nil {
			return fmt.Errorf("Redis did not become available: %w", err)
		}
		logger.Warn("waiting for Redis", "error", err)
		time.Sleep(time.Second)
	}

	app := server.New(cfg, store, logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx); err != nil {
		return err
	}
	return nil
}

func checkHealth() int {
	endpoint := os.Getenv("GAME_LOGIC_HEALTH_URL")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:8081/healthz"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, errors.New(response.Status))
		return 1
	}
	return 0
}
