package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 5 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 25 * time.Second
	maxMessageSize = 64 * 1024
	sendBufferSize = 64
)

type wsClient struct {
	player string
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
	if !slices.Contains(instance.engine.Players(), playerID) {
		writeError(w, http.StatusForbidden, "player is not in this game")
		return
	}
	if instance.engine.Status() == statusFinished {
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
		player: playerID, conn: conn, send: make(chan []byte, sendBufferSize), done: make(chan struct{}),
	}
	instance.opMu.Lock()
	old, out, startNow, started := a.attach(instance, client)
	if old != nil {
		old.shutdown()
	}
	go client.writePump()
	if startNow {
		a.publish(instance, out)
		a.startRunner(instance)
	} else if started {
		if !out.empty() {
			a.publish(instance, out)
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

// attach registra la conexión. La partida inicia cuando están conectados
// todos los jugadores de la lista que envió el Lobby.
func (a *App) attach(instance *gameInstance, client *wsClient) (old *wsClient, out outcome, startNow, started bool) {
	instance.mu.Lock()
	defer instance.mu.Unlock()
	old = instance.clients[client.player]
	instance.clients[client.player] = client
	if instance.started {
		out, _ = instance.engine.SetConnected(client.player, true)
		return old, out, false, true
	}
	if len(instance.clients) == len(instance.players) {
		instance.started = true
		var err error
		out, err = instance.engine.Start()
		if err != nil {
			instance.started = false
			delete(instance.clients, client.player)
			client.shutdown()
			a.logger.Error("start match after all players connected", "game_id", instance.id, "error", err)
			return old, outcome{}, false, false
		}
		return old, out, true, true
	}
	return old, outcome{}, false, false
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
		instance.opMu.Lock()
		out, err := instance.engine.Apply(c.player, message)
		if err != nil {
			instance.opMu.Unlock()
			code, text := "invalid_action", err.Error()
			var actErr *actionError
			if errors.As(err, &actErr) {
				code, text = actErr.Code, actErr.Message
			}
			c.sendJSON(map[string]any{"type": "error", "code": code, "message": text})
			continue
		}
		a.publish(instance, out)
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
		a.publishWaiting(instance)
		return
	}
	out, err := instance.engine.SetConnected(client.player, false)
	if err != nil {
		a.logger.Error("mark player disconnected", "game_id", instance.id, "player_id", client.player, "error", err)
		return
	}
	a.publish(instance, out)
}

// publish persiste el estado, difunde los eventos y envía a cada conexión su vista.
func (a *App) publish(instance *gameInstance, out outcome) {
	if out.empty() {
		return
	}
	if err := a.save(instance); err != nil {
		a.logger.Error("persist game after state change", "game_id", instance.id, "error", err)
	}
	instance.mu.Lock()
	if out.roundEnded && instance.engine.Status() == statusRoundOver {
		instance.roundDue = time.Now().Add(a.cfg.RoundPause)
	}
	instance.mu.Unlock()
	clients := a.clients(instance)
	for _, event := range out.events {
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
	if out.gameEnded {
		a.reportResult(instance)
	}
}

func (a *App) sendView(instance *gameInstance, client *wsClient) {
	view, err := instance.engine.View(client.player)
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
		if _, ok := instance.clients[player.ID]; ok {
			connected = append(connected, map[string]string{"id": player.ID, "name": player.Name})
		}
	}
	instance.mu.Unlock()
	message := map[string]any{
		"type": "game_waiting", "gameId": instance.id, "mode": instance.mode, "connectedPlayers": connected,
		"requiredPlayers": len(instance.players),
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
