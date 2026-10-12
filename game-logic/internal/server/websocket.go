package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/game"
)

const (
	writeWait      = 5 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 25 * time.Second
	maxMessageSize = 64 * 1024
	sendBufferSize = 64
)

type wsClient struct {
	player game.PlayerID
	conn   *websocket.Conn
	send   chan []byte
	done   chan struct{}
	once   sync.Once
}

func (a *App) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	gameID, playerID, err := verifyGameToken(token, a.cfg.JWTSecret)
	if err != nil || gameID != r.PathValue("gameID") {
		writeError(w, http.StatusUnauthorized, "invalid game token")
		return
	}
	a.mu.RLock()
	instance := a.games[gameID]
	a.mu.RUnlock()
	if instance == nil {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	player := game.PlayerID(playerID)
	if !containsPlayer(instance.match.Players(), player) {
		writeError(w, http.StatusForbidden, "player is not in this game")
		return
	}
	if instance.match.Phase() == game.PhaseGameOver {
		writeError(w, http.StatusGone, "game is finished")
		return
	}
	upgrader := websocket.Upgrader{
		HandshakeTimeout: 5 * time.Second,
		CheckOrigin:      a.checkOrigin,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &wsClient{
		player: player, conn: conn, send: make(chan []byte, sendBufferSize), done: make(chan struct{}),
	}
	instance.opMu.Lock()
	old, events, startNow, started := a.attach(instance, client)
	if old != nil {
		old.shutdown()
	}
	go client.writePump()
	if startNow {
		a.publish(instance, events)
		a.startRunner(instance)
	} else if started {
		if len(events) != 0 {
			a.publish(instance, events)
		} else {
			a.sendView(instance, client)
		}
	} else {
		a.publishWaiting(instance)
	}
	instance.opMu.Unlock()
	a.maybeStartNextRound(instance)
	client.readPump(a, instance)
}

func (a *App) attach(instance *gameInstance, client *wsClient) (old *wsClient, events []game.Event, startNow, started bool) {
	instance.mu.Lock()
	defer instance.mu.Unlock()
	old = instance.clients[client.player]
	instance.clients[client.player] = client
	if instance.started {
		events, _ = instance.match.SetConnected(client.player, true)
		return old, events, false, true
	}
	if len(instance.clients) == game.NumPlayers {
		instance.started = true
		var err error
		events, err = instance.match.Start()
		if err != nil {
			instance.started = false
			delete(instance.clients, client.player)
			client.shutdown()
			a.logger.Error("start match after all players connected", "game_id", instance.id, "error", err)
			return old, nil, false, false
		}
		return old, events, true, true
	}
	return old, nil, false, false
}

func (c *wsClient) readPump(a *App, instance *gameInstance) {
	defer func() {
		a.detach(instance, c)
		c.shutdown()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		a.logger.Warn("set websocket read deadline", "game_id", instance.id, "error", err)
		return
	}
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived, 4001, 4003, 4008) {
				a.logger.Debug("websocket reader ended", "game_id", instance.id, "player_id", c.player, "error", err)
			}
			return
		}
		var action game.Action
		decoder := json.NewDecoder(strings.NewReader(string(message)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&action); err != nil {
			c.sendJSON(map[string]any{"type": "error", "code": "invalid_action", "message": "mensaje JSON inválido"})
			continue
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			c.sendJSON(map[string]any{"type": "error", "code": "invalid_action", "message": "se esperaba una sola acción por mensaje"})
			continue
		}
		action.Player = c.player
		instance.opMu.Lock()
		events, err := instance.match.Apply(action)
		if err != nil {
			instance.opMu.Unlock()
			code := string(game.CodeOf(err))
			if code == "" {
				code = "invalid_action"
			}
			messageText := err.Error()
			var gameErr *game.Error
			if errors.As(err, &gameErr) {
				messageText = gameErr.Message
			}
			c.sendJSON(map[string]any{"type": "error", "code": code, "message": messageText})
			continue
		}
		a.publish(instance, events)
		instance.opMu.Unlock()
	}
}

func (a *App) detach(instance *gameInstance, client *wsClient) {
	instance.opMu.Lock()
	defer instance.opMu.Unlock()
	instance.mu.Lock()
	if instance.clients[client.player] != client {
		instance.mu.Unlock()
		return
	}
	delete(instance.clients, client.player)
	started := instance.started
	instance.mu.Unlock()
	if !started {
		return
	}
	events, err := instance.match.SetConnected(client.player, false)
	if err != nil {
		a.logger.Error("mark player disconnected", "game_id", instance.id, "player_id", client.player, "error", err)
		return
	}
	a.publish(instance, events)
}

func (a *App) publish(instance *gameInstance, events []game.Event) {
	if len(events) == 0 {
		return
	}
	if err := a.save(instance); err != nil {
		a.logger.Error("persist game after state change", "game_id", instance.id, "error", err)
	}
	instance.mu.Lock()
	if hasEvent(events, game.EvRoundEnded) && instance.match.Phase() == game.PhaseRoundOver {
		instance.roundDue = time.Now().Add(a.cfg.RoundPause)
	}
	instance.mu.Unlock()
	clients := a.clients(instance)
	for _, event := range events {
		for _, client := range clients {
			if err := client.sendJSON(event); err != nil {
				a.logger.Warn("websocket consumer is too slow", "game_id", instance.id, "player_id", client.player)
				client.closeWith(4008, "slow consumer")
			}
		}
	}
	for _, client := range clients {
		a.sendView(instance, client)
	}
	for _, event := range events {
		if event.Type == game.EvGameEnded {
			a.reportResult(instance)
			break
		}
	}
}

func (a *App) publishViews(instance *gameInstance) {
	for _, client := range a.clients(instance) {
		a.sendView(instance, client)
	}
}

func (a *App) sendView(instance *gameInstance, client *wsClient) {
	view, err := instance.match.ViewFor(client.player)
	if err != nil {
		client.sendJSON(map[string]any{"type": "error", "code": "unknown_player", "message": "jugador desconocido"})
		return
	}
	if err := client.sendJSON(map[string]any{"type": "view", "view": view}); err != nil {
		client.closeWith(4008, "slow consumer")
	}
}

func (a *App) publishWaiting(instance *gameInstance) {
	instance.mu.Lock()
	connected := make([]map[string]string, 0, len(instance.clients))
	clients := make([]*wsClient, 0, len(instance.clients))
	for _, client := range instance.clients {
		clients = append(clients, client)
	}
	for _, player := range instance.players {
		if _, ok := instance.clients[game.PlayerID(player.ID)]; ok {
			connected = append(connected, map[string]string{"id": player.ID, "name": player.Name})
		}
	}
	instance.mu.Unlock()
	message := map[string]any{
		"type": "game_waiting", "gameId": instance.id, "connectedPlayers": connected,
		"requiredPlayers": game.NumPlayers,
	}
	for _, client := range clients {
		if err := client.sendJSON(message); err != nil {
			client.closeWith(4008, "slow consumer")
		}
	}
}

func (a *App) clients(instance *gameInstance) []*wsClient {
	instance.mu.Lock()
	defer instance.mu.Unlock()
	clients := make([]*wsClient, 0, len(instance.clients))
	for _, client := range instance.clients {
		clients = append(clients, client)
	}
	return clients
}

func (a *App) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	for _, allowed := range a.cfg.AllowedOrigins {
		if origin == allowed {
			return true
		}
	}
	if len(a.cfg.AllowedOrigins) != 0 {
		return false
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host == r.Host && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func (c *wsClient) sendJSON(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	select {
	case <-c.done:
		return errors.New("websocket is closed")
	case c.send <- data:
		return nil
	default:
		return errors.New("websocket send queue is full")
	}
}

func (c *wsClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case message := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				c.shutdown()
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				c.shutdown()
				return
			}
		case <-ticker.C:
			if err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
				c.shutdown()
				return
			}
		}
	}
}

func (c *wsClient) closeWith(code int, reason string) {
	c.once.Do(func() {
		_ = c.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(writeWait))
		close(c.done)
		_ = c.conn.Close()
	})
}

func (c *wsClient) shutdown() {
	c.once.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

func containsPlayer(players []game.PlayerID, id game.PlayerID) bool {
	for _, player := range players {
		if player == id {
			return true
		}
	}
	return false
}

func hasEvent(events []game.Event, eventType game.EventType) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
