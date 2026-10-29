package redisstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

// Pruebas contra un Redis real. Se omiten si TEST_REDIS_URL no está definido:
//
//	docker run --rm -d -p 127.0.0.1:16379:6379 redis:7.4.11-alpine
//	$env:TEST_REDIS_URL = "redis://127.0.0.1:16379/15"
func testStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("define TEST_REDIS_URL para probar contra Redis")
	}
	client, err := NewClient(url)
	if err != nil {
		t.Fatal(err)
	}
	prefix := fmt.Sprintf("bololo-test-%d:", time.Now().UnixNano())
	store := New(client, prefix)
	t.Cleanup(func() {
		ctx := context.Background()
		keys, _ := client.Keys(ctx, prefix+"*").Result()
		if len(keys) > 0 {
			client.Del(ctx, keys...)
		}
		_ = store.Close()
	})
	if err := store.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestStoreModeRoundTripAndIdempotency(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	players := []Player{{ID: "p_1", Name: "Ana"}, {ID: "p_2", Name: "Beto"}}
	record := Record{GameID: "g_1", LobbyID: "l_1", Mode: "CHAMELEON", Players: players, Snapshot: []byte(`{"v":1}`), Status: "WAITING"}
	if err := store.Create(ctx, record); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, record); err != nil {
		t.Fatalf("crear de nuevo la misma partida debe ser idempotente: %v", err)
	}
	other := record
	other.Mode = "CLASSIC"
	if err := store.Create(ctx, other); !errors.Is(err, ErrGameConflict) {
		t.Fatalf("mismo id con otro modo: %v", err)
	}
	if err := store.Save(ctx, "g_1", []byte(`{"v":2}`), "PLAYING"); err != nil {
		t.Fatal(err)
	}
	// Sin modo explícito se guarda CLASSIC.
	legacy := Record{GameID: "g_2", LobbyID: "l_2", Players: players, Snapshot: []byte(`{}`), Status: "WAITING"}
	if err := store.Create(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	records, err := store.LoadActive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Record{}
	for _, r := range records {
		byID[r.GameID] = r
	}
	if got := byID["g_1"]; got.Mode != "CHAMELEON" || string(got.Snapshot) != `{"v":2}` || got.Status != "PLAYING" || len(got.Players) != 2 {
		t.Fatalf("g_1: %+v", got)
	}
	if byID["g_2"].Mode != "CLASSIC" {
		t.Fatalf("g_2: %+v", byID["g_2"])
	}
	if err := store.Finish(ctx, "g_1"); err != nil {
		t.Fatal(err)
	}
	records, _ = store.LoadActive(ctx)
	if len(records) != 1 || records[0].GameID != "g_2" {
		t.Fatalf("tras terminar g_1 quedan: %+v", records)
	}
	if err := store.Save(ctx, "nope", nil, "x"); err == nil {
		t.Fatal("guardar una partida inexistente debe fallar")
	}
}
