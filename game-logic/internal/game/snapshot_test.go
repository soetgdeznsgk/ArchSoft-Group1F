package game

import (
	"encoding/json"
	"math/rand/v2"
	"testing"
	"time"
)

func TestSnapshotRoundTripPreservesMatchState(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)}
	config := Config{Rand: rand.New(rand.NewPCG(31, 47)), Clock: clock}
	match, err := NewMatch(testPlayers, config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := match.Start(); err != nil {
		t.Fatal(err)
	}
	player := match.CurrentPlayer()
	if _, err := match.Apply(Action{Type: ActDraw, Player: player, Source: SourceHidden}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := match.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreMatch(snapshot, Config{Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	if match.Phase() != restored.Phase() || match.Round() != restored.Round() ||
		match.CurrentPlayer() != restored.CurrentPlayer() {
		t.Fatalf("restored match differs: phase %q/%q round %d/%d turn %q/%q",
			match.Phase(), restored.Phase(), match.Round(), restored.Round(),
			match.CurrentPlayer(), restored.CurrentPlayer())
	}
	for _, id := range testPlayers {
		want, err := match.ViewFor(id)
		if err != nil {
			t.Fatal(err)
		}
		got, err := restored.ViewFor(id)
		if err != nil {
			t.Fatal(err)
		}
		wantJSON, _ := json.Marshal(want)
		gotJSON, _ := json.Marshal(got)
		if string(wantJSON) != string(gotJSON) {
			t.Fatalf("restored view for %q differs\nwant %s\ngot  %s", id, wantJSON, gotJSON)
		}
	}
}

func TestRestoreMatchRejectsUnsupportedAndInvalidSnapshots(t *testing.T) {
	for _, snapshot := range [][]byte{
		[]byte(`{"version":2}`),
		[]byte(`{"version":1,"unknown":true}`),
		[]byte(`{} {}`),
	} {
		if _, err := RestoreMatch(snapshot, Config{}); err == nil {
			t.Fatalf("RestoreMatch(%s) succeeded", snapshot)
		}
	}
}

func TestMarkAllDisconnectedDoesNotAdvanceRound(t *testing.T) {
	match, err := NewMatch(testPlayers, Config{Rand: rand.New(rand.NewPCG(31, 47))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := match.Start(); err != nil {
		t.Fatal(err)
	}
	events := match.MarkAllDisconnected()
	if len(events) != NumPlayers {
		t.Fatalf("disconnect events = %d, want %d", len(events), NumPlayers)
	}
	if match.Phase() != PhaseAwaitingDraw {
		t.Fatalf("phase changed to %q while marking recovered sockets offline", match.Phase())
	}
	for _, id := range testPlayers {
		view, err := match.ViewFor(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, player := range view.Players {
			if player.Connected {
				t.Fatalf("player %q is still marked connected after recovery", player.ID)
			}
		}
	}
}
