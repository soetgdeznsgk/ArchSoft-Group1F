package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/game"
	redisstore "github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/store/redis"
)

const (
	statusWaiting   = "WAITING"
	statusPlaying   = "PLAYING"
	statusRoundOver = "ROUND_OVER"
	statusFinished  = "FINISHED"
)

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type StateStore interface {
	Ping(context.Context) error
	Create(context.Context, redisstore.Record) error
	Save(context.Context, string, []byte, string) error
	LoadActive(context.Context) ([]redisstore.Record, error)
	Finish(context.Context, string) error
	Close() error
}

type App struct {
	cfg        Config
	store      StateStore
	httpClient *http.Client
	logger     *slog.Logger
	ctx        context.Context
	cancel     context.CancelFunc

	mu       sync.RWMutex
	games    map[string]*gameInstance
	failures map[string]error
}

type gameInstance struct {
	id      string
	lobbyID string
	players []redisstore.Player
	match   *game.Match

	opMu            sync.Mutex
	mu              sync.Mutex
	clients         map[game.PlayerID]*wsClient
	started         bool
	cancel          context.CancelFunc
	roundDue        time.Time
	callbackRunning bool
}

type createGameRequest struct {
	GameID  string              `json:"gameId"`
	LobbyID string              `json:"lobbyId"`
	Players []redisstore.Player `json:"players"`
}

type createGameResponse struct {
	GameID string `json:"gameId"`
	Status string `json:"status"`
	WSPath string `json:"wsPath"`
}

type gameStatusResponse struct {
	GameID  string             `json:"gameId"`
	LobbyID string             `json:"lobbyId"`
	Status  string             `json:"status"`
	Round   int                `json:"round"`
	Players []gameStatusPlayer `json:"players"`
	Winners []game.PlayerID    `json:"winners,omitempty"`
}

type gameStatusPlayer struct {
	ID        game.PlayerID `json:"id"`
	Name      string        `json:"name"`
	Connected bool          `json:"connected"`
	HandSize  int           `json:"handSize"`
}

func New(cfg Config, store StateStore, logger *slog.Logger) *App {
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &App{
		cfg: cfg, store: store, logger: logger,
		httpClient: &http.Client{Timeout: cfg.CallbackTimeout},
		ctx:        ctx, cancel: cancel,
		games:    make(map[string]*gameInstance),
		failures: make(map[string]error),
	}
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("POST /internal/games", a.createGame)
	mux.HandleFunc("GET /internal/games/{gameID}", a.getGame)
	mux.HandleFunc("GET /ws/games/{gameID}", a.serveWebSocket)
	return mux
}

func (a *App) RestoreActive(ctx context.Context) error {
	records, err := a.store.LoadActive(ctx)
	if err != nil {
		return fmt.Errorf("load active games: %w", err)
	}
	for _, record := range records {
		if !validID.MatchString(record.GameID) || !validID.MatchString(record.LobbyID) {
			return fmt.Errorf("invalid identifiers in Redis record for game %q", record.GameID)
		}
		ids := make([]game.PlayerID, len(record.Players))
		for i, player := range record.Players {
			ids[i] = game.PlayerID(player.ID)
		}
		match, err := game.RestoreMatch(record.Snapshot, game.Config{})
		if err != nil {
			return fmt.Errorf("restore game %q: %w", record.GameID, err)
		}
		if !samePlayerIDs(match.Players(), ids) {
			return fmt.Errorf("Redis roster does not match snapshot for game %q", record.GameID)
		}
		instance := &gameInstance{
			id: record.GameID, lobbyID: record.LobbyID, players: record.Players,
			match: match, clients: make(map[game.PlayerID]*wsClient),
			started: match.Phase() != game.PhaseWaiting,
		}
		if err := a.register(instance); err != nil {
			return fmt.Errorf("register restored game %q: %w", record.GameID, err)
		}
		if instance.started {
			if match.Phase() != game.PhaseGameOver {
				events := match.MarkAllDisconnected()
				if len(events) != 0 {
					if err := a.save(instance); err != nil {
						a.logger.Error("persist recovered disconnections", "game_id", instance.id, "error", err)
					}
				}
			}
			a.startRunner(instance)
			if match.Phase() == game.PhaseGameOver {
				a.reportResult(instance)
			}
		}
	}
	return nil
}

func (a *App) createGame(w http.ResponseWriter, r *http.Request) {
	if !a.authorizedService(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request createGameRequest
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "request must contain one JSON value")
		return
	}
	if !validID.MatchString(request.GameID) || !validID.MatchString(request.LobbyID) ||
		len(request.Players) != game.NumPlayers {
		writeError(w, http.StatusBadRequest, "gameId, lobbyId and exactly four players are required")
		return
	}
	ids := make([]game.PlayerID, len(request.Players))
	seen := make(map[string]struct{}, len(request.Players))
	for i, player := range request.Players {
		if !validID.MatchString(player.ID) || strings.TrimSpace(player.Name) == "" || len(player.Name) > 20 {
			writeError(w, http.StatusBadRequest, "each player needs a valid id and a name of at most 20 characters")
			return
		}
		if _, exists := seen[player.ID]; exists {
			writeError(w, http.StatusBadRequest, "duplicate player id")
			return
		}
		seen[player.ID] = struct{}{}
		ids[i] = game.PlayerID(player.ID)
	}

	a.mu.Lock()
	if existing := a.games[request.GameID]; existing != nil {
		existing.mu.Lock()
		same := existing.lobbyID == request.LobbyID && samePlayers(existing.players, request.Players)
		existing.mu.Unlock()
		a.mu.Unlock()
		if !same {
			writeError(w, http.StatusConflict, "game id already belongs to another match")
			return
		}
		writeJSON(w, http.StatusOK, createGameResponse{
			GameID: request.GameID, Status: a.gameStatus(existing), WSPath: "/ws/games/" + request.GameID,
		})
		return
	}
	match, err := game.NewMatch(ids, game.Config{})
	if err != nil {
		a.mu.Unlock()
		writeError(w, http.StatusBadRequest, "invalid player roster")
		return
	}
	snapshot, err := match.Snapshot()
	if err != nil {
		a.mu.Unlock()
		a.logger.Error("snapshot new game", "game_id", request.GameID, "error", err)
		writeError(w, http.StatusInternalServerError, "could not initialize game")
		return
	}
	record := redisstore.Record{
		GameID: request.GameID, LobbyID: request.LobbyID, Players: request.Players,
		Snapshot: snapshot, Status: statusWaiting,
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.StoreTimeout)
	err = a.store.Create(ctx, record)
	cancel()
	if err != nil {
		a.mu.Unlock()
		a.logger.Error("persist new game", "game_id", request.GameID, "error", err)
		status := http.StatusServiceUnavailable
		if errors.Is(err, redisstore.ErrGameConflict) {
			status = http.StatusConflict
		} else {
			a.recordFailure(request.GameID, err)
		}
		writeError(w, status, "could not persist game")
		return
	}
	instance := &gameInstance{
		id: request.GameID, lobbyID: request.LobbyID, players: request.Players,
		match: match, clients: make(map[game.PlayerID]*wsClient),
	}
	a.games[request.GameID] = instance
	a.mu.Unlock()
	a.clearFailure(request.GameID)
	writeJSON(w, http.StatusCreated, createGameResponse{
		GameID: request.GameID, Status: statusWaiting, WSPath: "/ws/games/" + request.GameID,
	})
}

func (a *App) getGame(w http.ResponseWriter, r *http.Request) {
	if !a.authorizedService(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	a.mu.RLock()
	instance := a.games[r.PathValue("gameID")]
	a.mu.RUnlock()
	if instance == nil {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	response, err := a.gameStatusResponse(instance)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read game state")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.StoreTimeout)
	defer cancel()
	if err := a.store.Ping(ctx); err != nil {
		a.logger.Warn("health check failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "redis": "unavailable"})
		return
	}
	a.mu.RLock()
	degraded := len(a.failures) != 0
	a.mu.RUnlock()
	if degraded {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "redis": "ok"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "redis": "ok"})
}

func (a *App) authorizedService(r *http.Request) bool {
	provided := []byte(r.Header.Get("X-Service-Token"))
	return len(provided) != 0 && hmac.Equal(provided, a.cfg.ServiceSecret)
}

func (a *App) register(instance *gameInstance) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.games[instance.id]; exists {
		return fmt.Errorf("duplicate game id")
	}
	a.games[instance.id] = instance
	return nil
}

func (a *App) gameStatus(instance *gameInstance) string {
	switch instance.match.Phase() {
	case game.PhaseWaiting:
		return statusWaiting
	case game.PhaseRoundOver:
		return statusRoundOver
	case game.PhaseGameOver:
		return statusFinished
	default:
		return statusPlaying
	}
}

func (a *App) gameStatusResponse(instance *gameInstance) (gameStatusResponse, error) {
	response := gameStatusResponse{
		GameID: instance.id, LobbyID: instance.lobbyID, Status: a.gameStatus(instance),
		Round: instance.match.Round(),
	}
	instance.mu.Lock()
	connected := make(map[game.PlayerID]bool, len(instance.clients))
	for id := range instance.clients {
		connected[id] = true
	}
	instance.mu.Unlock()
	for _, player := range instance.players {
		view, err := instance.match.ViewFor(game.PlayerID(player.ID))
		if err != nil {
			return gameStatusResponse{}, err
		}
		response.Players = append(response.Players, gameStatusPlayer{
			ID: game.PlayerID(player.ID), Name: player.Name, Connected: connected[game.PlayerID(player.ID)],
			HandSize: playerCardCount(view, game.PlayerID(player.ID)),
		})
	}
	if final := instance.match.Final(); final != nil {
		response.Winners = final.Winners
	}
	return response, nil
}

func playerCardCount(view game.PlayerView, player game.PlayerID) int {
	for _, summary := range view.Players {
		if summary.ID == player {
			return summary.CardCount
		}
	}
	return 0
}

func (a *App) save(instance *gameInstance) error {
	snapshot, err := instance.match.Snapshot()
	if err != nil {
		a.recordFailure(instance.id, err)
		return fmt.Errorf("snapshot game %q: %w", instance.id, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.StoreTimeout)
	defer cancel()
	if err := a.store.Save(ctx, instance.id, snapshot, a.gameStatus(instance)); err != nil {
		a.recordFailure(instance.id, err)
		return err
	}
	a.clearFailure(instance.id)
	return nil
}

func (a *App) recordFailure(gameID string, err error) {
	a.mu.Lock()
	a.failures[gameID] = err
	a.mu.Unlock()
	a.logger.Error("game state persistence failed", "game_id", gameID, "error", err)
}

func (a *App) clearFailure(gameID string) {
	a.mu.Lock()
	delete(a.failures, gameID)
	a.mu.Unlock()
}

func (a *App) closeClients() {
	a.cancel()
	a.mu.RLock()
	instances := make([]*gameInstance, 0, len(a.games))
	for _, instance := range a.games {
		instances = append(instances, instance)
	}
	a.mu.RUnlock()
	for _, instance := range instances {
		instance.mu.Lock()
		for _, client := range instance.clients {
			client.shutdown()
		}
		if instance.cancel != nil {
			instance.cancel()
		}
		instance.mu.Unlock()
	}
}

func samePlayers(left, right []redisstore.Player) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func samePlayerIDs(left, right []game.PlayerID) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("write JSON response", "error", err)
	}
}

func decodeJWTPart(part string, target any) error {
	data, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func verifyGameToken(token string, secret []byte) (string, string, error) {
	if len(token) == 0 || len(token) > 8192 {
		return "", "", errors.New("invalid token length")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", "", errors.New("malformed token")
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if err := decodeJWTPart(parts[0], &header); err != nil || header.Algorithm != "HS256" {
		return "", "", errors.New("unsupported token algorithm")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", "", errors.New("malformed token signature")
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return "", "", errors.New("invalid token signature")
	}
	var claims struct {
		GameID   string      `json:"gameId"`
		PlayerID string      `json:"playerId"`
		IssuedAt json.Number `json:"iat"`
		Expires  json.Number `json:"exp"`
	}
	if err := decodeJWTPart(parts[1], &claims); err != nil {
		return "", "", errors.New("malformed token claims")
	}
	issuedAt, issuedAtErr := claims.IssuedAt.Int64()
	exp, err := claims.Expires.Int64()
	now := time.Now().Unix()
	if issuedAtErr != nil || err != nil || issuedAt > now+60 || exp <= now || exp <= issuedAt ||
		claims.GameID == "" || claims.PlayerID == "" {
		return "", "", errors.New("expired or incomplete token")
	}
	return claims.GameID, claims.PlayerID, nil
}
