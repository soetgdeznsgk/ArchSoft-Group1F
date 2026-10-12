package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

func (a *App) Run(ctx context.Context) error {
	if err := a.RestoreActive(ctx); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", a.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", a.cfg.ListenAddr, err)
	}
	httpServer := &http.Server{
		Handler:           a.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()
	select {
	case <-ctx.Done():
		a.closeClients()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
		return nil
	case err := <-serveErr:
		a.closeClients()
		_ = httpServer.Close()
		return fmt.Errorf("serve Game Logic: %w", err)
	}
}
