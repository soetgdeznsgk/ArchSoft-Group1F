package server

import (
	"context"
	"encoding/json"
	"fmt"
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
	if instance.resultRunning {
		instance.mu.Unlock()
		return
	}
	instance.resultRunning = true
	instance.mu.Unlock()
	go func() {
		defer func() {
			instance.mu.Lock()
			instance.resultRunning = false
			instance.mu.Unlock()
		}()
		if err := a.publishResult(instance); err != nil {
			a.logger.Error("publish final game result", "game_id", instance.id, "error", err)
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

// gameEnded es el cuerpo del evento game.ended (docs/game-logic-design.md).
type gameEnded struct {
	GameID     string   `json:"gameId"`
	LobbyID    string   `json:"lobbyId"`
	Mode       string   `json:"mode"`
	WinnerID   string   `json:"winnerId,omitempty"`
	Winners    []string `json:"winners,omitempty"`
	EndReason  string   `json:"endReason"`
	Standings  any      `json:"standings"`
	FinishedAt string   `json:"finishedAt"`
}

// publishResult publica game.ended. La partida solo se marca terminada en Redis
// cuando RabbitMQ confirmó el mensaje; si Game Logic se reinicia antes,
// RestoreActive lo vuelve a publicar y el Lobby lo descarta por gameId.
func (a *App) publishResult(instance *gameInstance) error {
	final := instance.engine.Final()
	if final == nil {
		return fmt.Errorf("game %q ended without a final result", instance.id)
	}
	event := gameEnded{
		GameID: instance.id, LobbyID: instance.lobbyID, Mode: instance.mode,
		Winners: final.Winners, EndReason: final.EndReason, Standings: final.Standings,
		FinishedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if len(final.Winners) == 1 {
		event.WinnerID = final.Winners[0]
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal final result: %w", err)
	}
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		ctx, cancel := context.WithTimeout(a.ctx, a.cfg.PublishTimeout)
		lastErr = a.results.PublishResult(ctx, instance.id, body)
		cancel()
		if lastErr == nil {
			return nil
		}
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
	return fmt.Errorf("result publication failed after retries: %w", lastErr)
}

func retryDelay(attempt int) time.Duration {
	return time.Duration(1<<attempt) * time.Second
}
