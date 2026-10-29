package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

func (a *App) startRunner(instance *gameInstance) {
	instance.mu.Lock()
	if instance.cancel != nil {
		instance.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	instance.cancel = cancel
	if instance.engine.Status() == statusRoundOver && instance.roundDue.IsZero() {
		instance.roundDue = time.Now().Add(a.cfg.RoundPause)
	}
	instance.mu.Unlock()
	go a.runGame(ctx, instance)
}

func (a *App) runGame(ctx context.Context, instance *gameInstance) {
	ticker := time.NewTicker(a.cfg.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			switch instance.engine.Status() {
			case statusFinished:
				return
			case statusRoundOver:
				a.maybeStartNextRound(instance)
				continue
			}
			instance.opMu.Lock()
			a.publish(instance, instance.engine.Tick())
			instance.opMu.Unlock()
		}
	}
}

func (a *App) maybeStartNextRound(instance *gameInstance) {
	instance.opMu.Lock()
	defer instance.opMu.Unlock()
	instance.mu.Lock()
	if instance.engine.Status() != statusRoundOver || instance.roundDue.IsZero() ||
		time.Now().Before(instance.roundDue) || len(instance.clients) == 0 {
		instance.mu.Unlock()
		return
	}
	instance.roundDue = time.Time{}
	out, err := instance.engine.StartNextRound()
	instance.mu.Unlock()
	if err != nil {
		a.logger.Error("start next game round", "game_id", instance.id, "error", err)
		return
	}
	a.publish(instance, out)
}

func (a *App) reportResult(instance *gameInstance) {
	instance.mu.Lock()
	if instance.callbackRunning {
		instance.mu.Unlock()
		return
	}
	instance.callbackRunning = true
	instance.mu.Unlock()
	go func() {
		defer func() {
			instance.mu.Lock()
			instance.callbackRunning = false
			instance.mu.Unlock()
		}()
		if err := a.postResult(instance); err != nil {
			a.logger.Error("report final game result to Lobby", "game_id", instance.id, "error", err)
			a.recordFailure(instance.id, err)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), a.cfg.StoreTimeout)
		defer cancel()
		if err := a.store.Finish(ctx, instance.id); err != nil {
			a.recordFailure(instance.id, err)
			a.logger.Error("mark game finished in Redis", "game_id", instance.id, "error", err)
			return
		}
		a.clearFailure(instance.id)
	}()
}

func (a *App) postResult(instance *gameInstance) error {
	final := instance.engine.Final()
	if final == nil {
		return fmt.Errorf("game %q ended without a final result", instance.id)
	}
	// Las posiciones dependen del modo: puntos/rondas en CLASSIC, colores en CHAMELEON.
	payload := struct {
		GameID    string   `json:"gameId"`
		Mode      string   `json:"mode"`
		WinnerID  string   `json:"winnerId,omitempty"`
		Winners   []string `json:"winners,omitempty"`
		EndReason string   `json:"endReason"`
		Standings any      `json:"standings"`
	}{
		GameID: instance.id, Mode: instance.mode, Winners: final.Winners, EndReason: final.EndReason,
		Standings: final.Standings,
	}
	if len(final.Winners) == 1 {
		payload.WinnerID = final.Winners[0]
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal final result: %w", err)
	}
	endpoint, err := url.JoinPath(a.cfg.LobbyCallbackURL, "internal", "lobbies", url.PathEscape(instance.lobbyID), "result")
	if err != nil {
		return fmt.Errorf("build Lobby callback URL: %w", err)
	}
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		requestCtx, cancel := context.WithTimeout(a.ctx, a.cfg.CallbackTimeout)
		request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err == nil {
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Service-Token", string(a.cfg.ServiceSecret))
			response, requestErr := a.httpClient.Do(request)
			err = requestErr
			if requestErr == nil {
				_, _ = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
				if response.StatusCode >= 200 && response.StatusCode < 300 {
					cancel()
					return nil
				}
				err = fmt.Errorf("Lobby callback returned HTTP %d", response.StatusCode)
				if response.StatusCode < 500 {
					cancel()
					return err
				}
			}
		}
		cancel()
		lastErr = err
		if attempt == 5 {
			break
		}
		timer := time.NewTimer(retryDelay(attempt))
		select {
		case <-timer.C:
		case <-a.ctx.Done():
			timer.Stop()
			return a.ctx.Err()
		}
	}
	return fmt.Errorf("result callback failed after retries: %w", lastErr)
}

func retryDelay(attempt int) time.Duration {
	return time.Duration(1<<attempt) * time.Second
}
