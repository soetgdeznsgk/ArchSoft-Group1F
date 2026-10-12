package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/game"
	redisstore "github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/store/redis"
)

type memoryStore struct {
	mu      sync.Mutex
	records map[string]redisstore.Record
}

func newMemoryStore() *memoryStore {
	return &memoryStore{records: make(map[string]redisstore.Record)}
}

func (s *memoryStore) Ping(context.Context) error { return nil }
func (s *memoryStore) Close() error               { return nil }

func (s *memoryStore) Create(_ context.Context, record redisstore.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.records[record.GameID]; ok {
		if existing.LobbyID != record.LobbyID || !samePlayers(existing.Players, record.Players) {
			return redisstore.ErrGameConflict
		}
		return nil
	}
	s.records[record.GameID] = record
	return nil
}

func (s *memoryStore) Save(_ context.Context, id string, snapshot []byte, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return fmt.Errorf("missing game %s", id)
	}
	record.Snapshot = append([]byte(nil), snapshot...)
	record.Status = status
	s.records[id] = record
	return nil
}

func (s *memoryStore) LoadActive(context.Context) ([]redisstore.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records := make([]redisstore.Record, 0, len(s.records))
	for _, record := range s.records {
		records = append(records, record)
	}
	return records, nil
}

func (s *memoryStore) Finish(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return fmt.Errorf("missing game %s", id)
	}
	record.Status = "finished"
	s.records[id] = record
	return nil
}

func TestCreateGameIsAuthenticatedAndIdempotent(t *testing.T) {
	store := newMemoryStore()
	app := testApp(store)
	handler := app.Handler()
	body := createBody(t, testPlayers())

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/internal/games", strings.NewReader(body)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized request status = %d", unauthorized.Code)
	}

	first := createRequest(handler, body, "service-secret")
	if first.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body: %s", first.Code, first.Body)
	}
	second := createRequest(handler, body, "service-secret")
	if second.Code != http.StatusOK {
		t.Fatalf("idempotent retry status = %d, body: %s", second.Code, second.Body)
	}
	store.mu.Lock()
	record, exists := store.records["game_1"]
	store.mu.Unlock()
	if !exists {
		t.Fatal("created game was not persisted")
	}
	if _, err := game.RestoreMatch(record.Snapshot, game.Config{}); err != nil {
		t.Fatalf("stored snapshot is not restorable: %v", err)
	}
}

func TestCreateGameRejectsConflictingRoster(t *testing.T) {
	store := newMemoryStore()
	app := testApp(store)
	handler := app.Handler()
	if response := createRequest(handler, createBody(t, testPlayers()), "service-secret"); response.Code != http.StatusCreated {
		t.Fatalf("initial create status = %d", response.Code)
	}
	players := testPlayers()
	players[0].ID = "different"
	response := createRequest(handler, createBody(t, players), "service-secret")
	if response.Code != http.StatusConflict {
		t.Fatalf("conflicting roster status = %d, body: %s", response.Code, response.Body)
	}
}

func TestRestoreActiveReloadsWaitingGame(t *testing.T) {
	store := newMemoryStore()
	app := testApp(store)
	if response := createRequest(app.Handler(), createBody(t, testPlayers()), "service-secret"); response.Code != http.StatusCreated {
		t.Fatalf("create status = %d", response.Code)
	}
	restarted := testApp(store)
	if err := restarted.RestoreActive(context.Background()); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/internal/games/game_1", nil)
	request.Header.Set("X-Service-Token", "service-secret")
	response := httptest.NewRecorder()
	restarted.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"WAITING"`) {
		t.Fatalf("restored status = %d, body: %s", response.Code, response.Body)
	}
}

func TestWebSocketStartsMatchAndSendsPrivateViews(t *testing.T) {
	store := newMemoryStore()
	app := testApp(store)
	httpServer := httptest.NewServer(app.Handler())
	defer func() {
		app.closeClients()
		httpServer.Close()
	}()
	body := createBody(t, testPlayers())
	if response := createRequest(app.Handler(), body, "service-secret"); response.Code != http.StatusCreated {
		t.Fatalf("create status = %d", response.Code)
	}

	players := testPlayers()
	connections := make([]*websocket.Conn, len(players))
	for i, player := range players {
		token := signGameToken("game_1", player.ID, "jwt-secret")
		url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/games/game_1?token=" + token
		conn, response, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatalf("connect player %s: %v (response %#v)", player.ID, err, response)
		}
		connections[i] = conn
		defer conn.Close()
	}

	views := make([]testView, len(connections))
	for i, conn := range connections {
		view := readView(t, conn, players[i].ID)
		views[i] = view
		if len(view.Hand) != 3 {
			t.Fatalf("player %s received %d cards in own view; want 3", players[i].ID, len(view.Hand))
		}
		if strings.TrimSpace(view.CurrentTurn) == "" {
			t.Fatal("initial player view omitted the current turn")
		}
	}
	var attacker int
	var currentTurn string
	for i, player := range players {
		view := views[i]
		if player.ID != view.CurrentTurn {
			attacker = i
			currentTurn = view.CurrentTurn
			break
		}
	}
	if err := connections[attacker].WriteJSON(map[string]any{
		"type": "draw", "player": currentTurn, "source": "hidden",
	}); err != nil {
		t.Fatal(err)
	}
	if err := connections[attacker].SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	for {
		_, data, err := connections[attacker].ReadMessage()
		if err != nil {
			t.Fatalf("read spoofed-action response: %v", err)
		}
		var response struct {
			Type string `json:"type"`
			Code string `json:"code"`
		}
		if err := json.Unmarshal(data, &response); err != nil {
			t.Fatal(err)
		}
		if response.Type == "error" {
			if response.Code != "not_your_turn" {
				t.Fatalf("spoofed action error code = %q, want not_your_turn", response.Code)
			}
			break
		}
	}
}

func TestVerifyGameTokenRejectsTamperedAndExpiredTokens(t *testing.T) {
	valid := signGameToken("game_1", "p_1", "jwt-secret")
	gameID, playerID, err := verifyGameToken(valid, []byte("jwt-secret"))
	if err != nil || gameID != "game_1" || playerID != "p_1" {
		t.Fatalf("valid token returned %q %q %v", gameID, playerID, err)
	}
	for _, token := range []string{valid + "x", signTokenClaims(`{"gameId":"game_1","playerId":"p_1","iat":1,"exp":2}`, "jwt-secret")} {
		if _, _, err := verifyGameToken(token, []byte("jwt-secret")); err == nil {
			t.Fatalf("verifyGameToken(%q) succeeded", token)
		}
	}
}

type testView struct {
	You         string `json:"you"`
	CurrentTurn string `json:"current_turn"`
	Hand        []any  `json:"hand"`
}

func readView(t *testing.T, conn *websocket.Conn, expectedPlayer string) testView {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read websocket message: %v", err)
		}
		var envelope struct {
			Type string          `json:"type"`
			View json.RawMessage `json:"view"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			t.Fatalf("decode websocket message: %v", err)
		}
		if envelope.Type != "view" {
			continue
		}
		var view testView
		if err := json.Unmarshal(envelope.View, &view); err != nil {
			t.Fatal(err)
		}
		if view.You != expectedPlayer {
			t.Fatalf("received view for %q, want %q", view.You, expectedPlayer)
		}
		return view
	}
}

func testApp(store StateStore) *App {
	return New(Config{
		ListenAddr: ":0", JWTSecret: []byte("jwt-secret"), ServiceSecret: []byte("service-secret"),
		LobbyCallbackURL: "http://127.0.0.1:1", TickInterval: 10 * time.Millisecond,
		RoundPause: 10 * time.Millisecond, StoreTimeout: time.Second, CallbackTimeout: time.Second,
	}, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func testPlayers() []redisstore.Player {
	return []redisstore.Player{
		{ID: "p_1", Name: "Ana"}, {ID: "p_2", Name: "Beto"},
		{ID: "p_3", Name: "Caro"}, {ID: "p_4", Name: "Dani"},
	}
}

func createBody(t *testing.T, players []redisstore.Player) string {
	t.Helper()
	data, err := json.Marshal(createGameRequest{GameID: "game_1", LobbyID: "lobby_1", Players: players})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func createRequest(handler http.Handler, body, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/internal/games", strings.NewReader(body))
	request.Header.Set("X-Service-Token", token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func signGameToken(gameID, playerID, secret string) string {
	now := time.Now().Unix()
	claims := fmt.Sprintf(`{"gameId":%q,"playerId":%q,"iat":%d,"exp":%d}`, gameID, playerID, now, now+3600)
	return signTokenClaims(claims, secret)
}

func signTokenClaims(claims, secret string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(claims))
	unsigned := header + "." + payload
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
