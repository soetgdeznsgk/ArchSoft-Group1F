package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/chameleon"
	redisstore "github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/store/redis"
)

// fakeLobby recibe el callback del resultado final.
type fakeLobby struct {
	mu     sync.Mutex
	bodies []map[string]any
	got    chan struct{}
}

func newFakeLobby(t *testing.T) (*fakeLobby, *httptest.Server) {
	lobby := &fakeLobby{got: make(chan struct{}, 4)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Service-Token") != "service-secret" || !strings.HasSuffix(r.URL.Path, "/internal/lobbies/lobby_1/result") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]any
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		lobby.mu.Lock()
		lobby.bodies = append(lobby.bodies, body)
		lobby.mu.Unlock()
		lobby.got <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return lobby, srv
}

func chameleonApp(store StateStore, lobbyURL string, duration time.Duration) *App {
	app := testApp(store)
	app.cfg.LobbyCallbackURL = lobbyURL
	app.cfg.Chameleon = chameleon.Config{
		GameDuration: duration, SpawnMin: 20 * time.Millisecond, SpawnMax: 40 * time.Millisecond,
		CardLifetimeMin: 600 * time.Millisecond, CardLifetimeMax: 600 * time.Millisecond,
		MaxBoardCards: 8, ChameleonChance: chameleon.NoChameleons,
	}
	return app
}

func chameleonBody(t *testing.T, mode string, players []redisstore.Player) string {
	t.Helper()
	data, err := json.Marshal(createGameRequest{GameID: "game_1", LobbyID: "lobby_1", Mode: mode, Players: players})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

type message map[string]any

// readUntil lee mensajes hasta que match devuelva true.
func readUntil(t *testing.T, conn *websocket.Conn, match func(message) bool) message {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	for {
		var msg message
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("leer websocket: %v", err)
		}
		if match(msg) {
			return msg
		}
	}
}

func isType(typ string) func(message) bool {
	return func(m message) bool { return m["type"] == typ }
}

func dial(t *testing.T, srv *httptest.Server, player string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/games/game_1?token=" + signGameToken("game_1", player, "jwt-secret")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestCreateGameValidatesModeAndPlayerCount(t *testing.T) {
	app := testApp(newMemoryStore())
	two := testPlayers()[:2]
	cases := []struct {
		name    string
		mode    string
		players []redisstore.Player
		status  int
	}{
		{"camaleón con 1", ModeChameleon, testPlayers()[:1], http.StatusBadRequest},
		{"camaleón con 5", ModeChameleon, append(testPlayers(), redisstore.Player{ID: "p_5", Name: "Eva"}), http.StatusBadRequest},
		{"clásico con 2", ModeClassic, two, http.StatusBadRequest},
		{"modo desconocido", "POKER", two, http.StatusBadRequest},
		{"camaleón con 2", ModeChameleon, two, http.StatusCreated},
		{"repetido idéntico", ModeChameleon, two, http.StatusOK},
		{"mismo id con otro modo", ModeClassic, testPlayers(), http.StatusConflict},
	}
	for _, tc := range cases {
		response := createRequest(app.Handler(), chameleonBody(t, tc.mode, tc.players), "service-secret")
		if response.Code != tc.status {
			t.Fatalf("%s: HTTP %d, se esperaba %d: %s", tc.name, response.Code, tc.status, response.Body)
		}
		if tc.status == http.StatusCreated && !strings.Contains(response.Body.String(), `"mode":"CHAMELEON"`) {
			t.Fatalf("respuesta sin modo: %s", response.Body)
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/internal/games/game_1", nil)
	request.Header.Set("X-Service-Token", "service-secret")
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if !strings.Contains(response.Body.String(), `"mode":"CHAMELEON"`) || !strings.Contains(response.Body.String(), `"status":"WAITING"`) {
		t.Fatalf("estado: %s", response.Body)
	}
}

// Las partidas anteriores a los modos (sin "mode") siguen siendo clásicas.
func TestCreateGameWithoutModeIsClassic(t *testing.T) {
	store := newMemoryStore()
	app := testApp(store)
	response := createRequest(app.Handler(), createBody(t, testPlayers()), "service-secret")
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"mode":"CLASSIC"`) {
		t.Fatalf("HTTP %d: %s", response.Code, response.Body)
	}
	if mode := store.get("game_1").Mode; mode != ModeClassic {
		t.Fatalf("modo guardado: %q", mode)
	}
}

// Flujo completo por WebSocket: espera, inicio, robo de la misma carta, fin
// con los 8 colores y callback al Lobby con el resultado del modo.
func TestChameleonGameOverWebSocket(t *testing.T) {
	store := newMemoryStore()
	lobby, lobbySrv := newFakeLobby(t)
	app := chameleonApp(store, lobbySrv.URL, time.Minute)
	srv := httptest.NewServer(app.Handler())
	defer srv.Close()
	defer app.closeClients()
	if r := createRequest(app.Handler(), chameleonBody(t, ModeChameleon, testPlayers()[:2]), "service-secret"); r.Code != http.StatusCreated {
		t.Fatalf("crear: %d %s", r.Code, r.Body)
	}

	ana := dial(t, srv, "p_1")
	waiting := readUntil(t, ana, isType("game_waiting"))
	if waiting["requiredPlayers"] != float64(2) || waiting["mode"] != ModeChameleon {
		t.Fatalf("espera: %v", waiting)
	}
	beto := dial(t, srv, "p_2")
	for _, conn := range []*websocket.Conn{ana, beto} {
		view := readUntil(t, conn, isType("view"))["view"].(map[string]any)
		if view["mode"] != ModeChameleon || view["phase"] != "playing" {
			t.Fatalf("vista inicial: %v", view)
		}
	}

	// Ambos piden la misma carta: solo uno la recibe.
	spawned := readUntil(t, beto, isType("card_spawned"))
	card := spawned["card"].(map[string]any)
	id := int(card["id"].(float64))
	if err := ana.WriteJSON(map[string]any{"type": "take", "card": id}); err != nil {
		t.Fatal(err)
	}
	taken := readUntil(t, beto, func(m message) bool { return m["type"] == "card_taken" })
	if taken["player"] != "p_1" {
		t.Fatalf("toma: %v", taken)
	}
	if err := beto.WriteJSON(map[string]any{"type": "take", "card": id}); err != nil {
		t.Fatal(err)
	}
	if e := readUntil(t, beto, isType("error")); e["code"] != "card_not_available" {
		t.Fatalf("error: %v", e)
	}
	// Mensajes inválidos o del otro modo se rechazan con código.
	if err := beto.WriteJSON(map[string]any{"type": "draw", "source": "hidden"}); err != nil {
		t.Fatal(err)
	}
	if e := readUntil(t, beto, isType("error")); e["code"] != "invalid_action" {
		t.Fatalf("error: %v", e)
	}

	// Ana toma solo colores que no tiene hasta reunir los 8.
	have := map[string]bool{card["color"].(string): true}
	for len(have) < chameleon.NumColors {
		msg := readUntil(t, ana, isType("card_spawned"))
		c := msg["card"].(map[string]any)
		if have[c["color"].(string)] {
			continue
		}
		if err := ana.WriteJSON(map[string]any{"type": "take", "card": int(c["id"].(float64))}); err != nil {
			t.Fatal(err)
		}
		have[c["color"].(string)] = true
	}
	ended := readUntil(t, beto, isType("game_ended"))
	final := ended["final"].(map[string]any)
	if final["reason"] != "all_colors" || final["winners"].([]any)[0] != "p_1" {
		t.Fatalf("fin: %v", ended)
	}

	select {
	case <-lobby.got:
	case <-time.After(5 * time.Second):
		t.Fatal("no llegó el resultado al Lobby")
	}
	lobby.mu.Lock()
	body := lobby.bodies[0]
	lobby.mu.Unlock()
	if body["mode"] != ModeChameleon || body["endReason"] != "all_colors" || body["winnerId"] != "p_1" || body["gameId"] != "game_1" {
		t.Fatalf("callback: %v", body)
	}
	standings := body["standings"].([]any)
	if first := standings[0].(map[string]any); first["player"] != "p_1" || first["colors"] != float64(8) || first["rank"] != float64(1) {
		t.Fatalf("posiciones: %v", standings)
	}
	waitFor(t, func() bool { return store.get("game_1").Status == "finished" })
	// Una partida terminada ya no acepta conexiones.
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/games/game_1?token=" + signGameToken("game_1", "p_2", "jwt-secret")
	if _, resp, err := websocket.DefaultDialer.Dial(url, nil); err == nil || resp.StatusCode != http.StatusGone {
		t.Fatalf("conexión tras el fin: %v", err)
	}
}

// Sin nadie que complete los colores, la partida termina por tiempo.
func TestChameleonTimeUpReportsResult(t *testing.T) {
	lobby, lobbySrv := newFakeLobby(t)
	app := chameleonApp(newMemoryStore(), lobbySrv.URL, 300*time.Millisecond)
	srv := httptest.NewServer(app.Handler())
	defer srv.Close()
	defer app.closeClients()
	createRequest(app.Handler(), chameleonBody(t, ModeChameleon, testPlayers()[:2]), "service-secret")
	ana := dial(t, srv, "p_1")
	dial(t, srv, "p_2")
	ended := readUntil(t, ana, isType("game_ended"))
	if ended["final"].(map[string]any)["reason"] != "time_up" {
		t.Fatalf("fin: %v", ended)
	}
	select {
	case <-lobby.got:
	case <-time.After(5 * time.Second):
		t.Fatal("no llegó el resultado al Lobby")
	}
	lobby.mu.Lock()
	defer lobby.mu.Unlock()
	if body := lobby.bodies[0]; body["endReason"] != "time_up" || body["winnerId"] != nil {
		t.Fatalf("callback: %v", body)
	}
}

// Tras un reinicio, la partida en curso se restaura con el motor de su modo.
func TestRestoreActiveChameleonGame(t *testing.T) {
	store := newMemoryStore()
	app := chameleonApp(store, "http://127.0.0.1:1", time.Minute)
	srv := httptest.NewServer(app.Handler())
	createRequest(app.Handler(), chameleonBody(t, ModeChameleon, testPlayers()[:3]), "service-secret")
	ana := dial(t, srv, "p_1")
	dial(t, srv, "p_2")
	dial(t, srv, "p_3")
	spawned := readUntil(t, ana, isType("card_spawned"))
	if err := ana.WriteJSON(map[string]any{"type": "take", "card": spawned["card"].(map[string]any)["id"]}); err != nil {
		t.Fatal(err)
	}
	readUntil(t, ana, func(m message) bool { return m["type"] == "card_taken" && m["player"] == "p_1" })
	app.closeClients()
	srv.Close()

	// El store se lee con su mutex: los sockets que se cierran aún pueden guardar desconexiones.
	if mode := store.get("game_1").Mode; mode != ModeChameleon {
		t.Fatalf("modo en el registro: %q", mode)
	}
	restored := chameleonApp(store, "http://127.0.0.1:1", time.Minute)
	if err := restored.RestoreActive(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer restored.closeClients()
	status, err := restored.gameStatusResponse(restored.games["game_1"])
	if err != nil {
		t.Fatal(err)
	}
	if status.Mode != ModeChameleon || status.Status != statusPlaying || len(status.Players) != 3 || status.Players[0].HandSize != 1 {
		t.Fatalf("estado restaurado: %+v", status)
	}
	// Un registro con un modo inválido detiene el arranque en lugar de corromper la partida.
	store.put(redisstore.Record{GameID: "game_2", LobbyID: "lobby_2", Mode: "POKER", Players: testPlayers(), Snapshot: []byte("{}")})
	if err := chameleonApp(store, "", time.Minute).RestoreActive(context.Background()); err == nil {
		t.Fatal("se esperaba error por modo desconocido")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("la condición no se cumplió a tiempo")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
