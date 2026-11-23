package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/chameleon"
)

// Escenario 4 (Limited View). El servidor manda a cada jugador solo lo suyo y
// la identidad sale del token, nunca del cliente. Estas pruebas cubren los
// casos de autorización del WebSocket y la identidad que pedía ampliar la
// tarjeta: token de otra partida, token expirado, token fuera del roster y
// playerId falso en una acción (clásico y camaleón).

// signExpiredGameToken firma un token RS256 ya vencido (exp en el pasado). La
// firma es válida; lo que debe fallar es la verificación temporal.
func signExpiredGameToken(gameID, playerID string) string {
	past := time.Now().Add(-2 * time.Hour).Unix()
	claims := fmt.Sprintf(`{"gameId":%q,"playerId":%q,"iat":%d,"exp":%d}`, gameID, playerID, past, past+60)
	return signTokenClaims(claims, testGameKey)
}

// dialStatus intenta abrir el WebSocket y devuelve el código HTTP del handshake.
// Para un rechazo, websocket.Dial falla y el código viaja en la respuesta.
func dialStatus(t *testing.T, srv *httptest.Server, gameID, token string) int {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/games/" + gameID + "?token=" + token
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if conn != nil {
		conn.Close()
	}
	if err == nil {
		t.Fatalf("se esperaba rechazo del handshake, pero conectó")
	}
	if resp == nil {
		t.Fatalf("handshake sin respuesta HTTP: %v", err)
	}
	return resp.StatusCode
}

// TestWebSocketRejectsTokensFromAnotherGameAndRoster cubre tres rechazos del
// handshake: un token firmado para otra partida, un token vencido y un token
// válido cuyo jugador no está en el roster de esta partida.
func TestWebSocketRejectsTokensFromAnotherGameAndRoster(t *testing.T) {
	store := newMemoryStore()
	app := testApp(store)
	srv := httptest.NewServer(app.Handler())
	defer srv.Close()
	defer app.closeClients()
	if r := createRequest(app.Handler(), createBody(t, testPlayers()), "service-secret"); r.Code != http.StatusCreated {
		t.Fatalf("crear: %d %s", r.Code, r.Body)
	}

	cases := []struct {
		name   string
		gameID string
		token  string
		status int
	}{
		{
			name:   "token de otra partida",
			gameID: "game_1",
			token:  signGameToken("game_otro", "p_1"), // firma válida, gameId distinto
			status: http.StatusUnauthorized,
		},
		{
			name:   "token expirado",
			gameID: "game_1",
			token:  signExpiredGameToken("game_1", "p_1"),
			status: http.StatusUnauthorized,
		},
		{
			name:   "jugador fuera del roster",
			gameID: "game_1",
			token:  signGameToken("game_1", "intruso"), // válido pero no está en la lista del Lobby
			status: http.StatusForbidden,
		},
	}
	for _, tc := range cases {
		if got := dialStatus(t, srv, tc.gameID, tc.token); got != tc.status {
			t.Fatalf("%s: handshake HTTP %d, se esperaba %d", tc.name, got, tc.status)
		}
	}
}

// TestChameleonActionUsesTokenIdentityNotMessage demuestra que en camaleón,
// igual que en clásico, el servidor atribuye la acción al jugador del token y
// descarta el campo "player" del mensaje. Ana manda un take diciendo ser Beto;
// el robo debe quedar a nombre de Ana.
func TestChameleonActionUsesTokenIdentityNotMessage(t *testing.T) {
	store := newMemoryStore()
	app, _ := chameleonApp(store, time.Minute)
	srv := httptest.NewServer(app.Handler())
	defer srv.Close()
	defer app.closeClients()
	if r := createRequest(app.Handler(), chameleonBody(t, ModeChameleon, testPlayers()[:2]), "service-secret"); r.Code != http.StatusCreated {
		t.Fatalf("crear: %d %s", r.Code, r.Body)
	}

	ana := dial(t, srv, "p_1")
	beto := dial(t, srv, "p_2")
	for _, conn := range []*websocket.Conn{ana, beto} {
		readUntil(t, conn, isType("view"))
	}

	spawned := readUntil(t, beto, isType("card_spawned"))
	id := int(spawned["card"].(map[string]any)["id"].(float64))
	// Ana envía el take pero afirma ser "p_2" (Beto) en el cuerpo del mensaje.
	if err := ana.WriteJSON(map[string]any{"type": "take", "player": "p_2", "card": id}); err != nil {
		t.Fatal(err)
	}
	taken := readUntil(t, beto, isType("card_taken"))
	if taken["player"] != "p_1" {
		t.Fatalf("el robo se atribuyó a %v; debe ser p_1 (identidad del token), no el player del mensaje", taken["player"])
	}
}

// TestChameleonViewIsPublicByDesign deja por escrito la decisión del modo
// camaleón: las manos de todos son públicas a propósito (es la mecánica del
// juego, ver los colores que reúne cada quien), así que ViewFor expone las
// cartas de todos. El límite de "Limited View" aquí no es ocultar manos sino
// que "you" es la identidad del token: nadie puede pedir la vista de otro.
func TestChameleonViewIsPublicByDesign(t *testing.T) {
	players := []string{"p_1", "p_2", "p_3"}
	m, err := chameleon.New(players, chameleon.Config{
		GameDuration:    time.Minute,
		SpawnMin:        20 * time.Millisecond,
		SpawnMax:        40 * time.Millisecond,
		CardLifetimeMin: 600 * time.Millisecond,
		CardLifetimeMax: 600 * time.Millisecond,
		MaxBoardCards:   8,
		ChameleonChance: chameleon.NoChameleons,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(); err != nil {
		t.Fatal(err)
	}

	view, err := m.ViewFor("p_1")
	if err != nil {
		t.Fatal(err)
	}
	if view.You != "p_1" {
		t.Fatalf(`la vista identifica a "you" como %q, debe ser la del token "p_1"`, view.You)
	}
	// Público por diseño: la vista incluye a todos los jugadores y su recuento
	// de colores. Si esto cambiara a ocultar manos ajenas, sería un cambio de
	// reglas del modo, no una corrección de seguridad.
	if len(view.Players) != len(players) {
		t.Fatalf("la vista debe listar a los %d jugadores, listó %d", len(players), len(view.Players))
	}

	// Pedir la vista de un jugador desconocido falla: no se puede espiar a otro.
	if _, err := m.ViewFor("intruso"); err == nil {
		t.Fatal("ViewFor aceptó un jugador que no pertenece a la partida")
	}
}
