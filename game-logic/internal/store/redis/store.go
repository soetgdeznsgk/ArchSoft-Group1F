package redisstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const finishedTTL = time.Hour

var ErrGameConflict = errors.New("game id already belongs to another match")

type Player struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Record struct {
	GameID  string
	LobbyID string
	// Mode es el modo de juego (CLASSIC o CHAMELEON). Decide qué motor
	// restaura el snapshot. Vacío en registros anteriores a los modos (= CLASSIC).
	Mode     string
	Players  []Player
	Snapshot []byte
	Status   string
}

type Store struct {
	client    *redis.Client
	keyPrefix string
}

func New(client *redis.Client, keyPrefix string) *Store {
	if keyPrefix == "" {
		keyPrefix = "bololo:"
	}
	return &Store{client: client, keyPrefix: keyPrefix}
}

func NewClient(redisURL string) (*redis.Client, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	return redis.NewClient(options), nil
}

func (s *Store) Ping(ctx context.Context) error {
	if err := s.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping Redis: %w", err)
	}
	return nil
}

func (s *Store) Create(ctx context.Context, record Record) error {
	players, err := json.Marshal(record.Players)
	if err != nil {
		return fmt.Errorf("encode players for game %q: %w", record.GameID, err)
	}
	script := redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
  if redis.call("HGET", KEYS[1], "lobby_id") == ARGV[1]
    and redis.call("HGET", KEYS[1], "players") == ARGV[2]
    and (redis.call("HGET", KEYS[1], "mode") or "CLASSIC") == ARGV[7]
    and redis.call("HGET", KEYS[1], "status") ~= "finished" then
    return 0
  end
  return -1
end
redis.call("HSET", KEYS[1],
  "lobby_id", ARGV[1],
  "players", ARGV[2],
  "snapshot", ARGV[3],
  "status", ARGV[4],
  "updated_at", ARGV[5],
  "mode", ARGV[7])
redis.call("SADD", KEYS[2], ARGV[6])
return 1
`)
	mode := record.Mode
	if mode == "" {
		mode = "CLASSIC"
	}
	result, err := script.Run(ctx, s.client, []string{s.gameKey(record.GameID), s.activeKey()},
		record.LobbyID, players, record.Snapshot, record.Status,
		time.Now().UTC().Format(time.RFC3339Nano), record.GameID, mode).Int()
	if err != nil {
		return fmt.Errorf("create game %q in Redis: %w", record.GameID, err)
	}
	if result < 0 {
		return ErrGameConflict
	}
	return nil
}

func (s *Store) Save(ctx context.Context, gameID string, snapshot []byte, status string) error {
	script := redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 0 then
  return 0
end
redis.call("HSET", KEYS[1], "snapshot", ARGV[1], "status", ARGV[2], "updated_at", ARGV[3])
redis.call("SADD", KEYS[2], ARGV[4])
return 1
`)
	result, err := script.Run(ctx, s.client, []string{s.gameKey(gameID), s.activeKey()},
		snapshot, status, time.Now().UTC().Format(time.RFC3339Nano), gameID).Int()
	if err != nil {
		return fmt.Errorf("save game %q in Redis: %w", gameID, err)
	}
	if result == 0 {
		return fmt.Errorf("save game %q in Redis: record does not exist", gameID)
	}
	return nil
}

func (s *Store) LoadActive(ctx context.Context) ([]Record, error) {
	ids, err := s.client.SMembers(ctx, s.activeKey()).Result()
	if err != nil {
		return nil, fmt.Errorf("list active games in Redis: %w", err)
	}
	records := make([]Record, 0, len(ids))
	for _, id := range ids {
		fields, err := s.client.HGetAll(ctx, s.gameKey(id)).Result()
		if err != nil {
			return nil, fmt.Errorf("load game %q from Redis: %w", id, err)
		}
		if len(fields) == 0 {
			if err := s.client.SRem(ctx, s.activeKey(), id).Err(); err != nil {
				return nil, fmt.Errorf("remove missing game %q from active set: %w", id, err)
			}
			continue
		}
		var players []Player
		if err := json.Unmarshal([]byte(fields["players"]), &players); err != nil {
			return nil, fmt.Errorf("decode players for game %q: %w", id, err)
		}
		records = append(records, Record{
			GameID: id, LobbyID: fields["lobby_id"], Mode: fields["mode"], Players: players,
			Snapshot: []byte(fields["snapshot"]), Status: fields["status"],
		})
	}
	return records, nil
}

func (s *Store) Finish(ctx context.Context, gameID string) error {
	script := redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 0 then
  return 0
end
redis.call("HSET", KEYS[1], "status", "finished", "updated_at", ARGV[1])
redis.call("SREM", KEYS[2], ARGV[2])
redis.call("EXPIRE", KEYS[1], ARGV[3])
return 1
`)
	result, err := script.Run(ctx, s.client, []string{s.gameKey(gameID), s.activeKey()},
		time.Now().UTC().Format(time.RFC3339Nano), gameID, int(finishedTTL.Seconds())).Int()
	if err != nil {
		return fmt.Errorf("finish game %q in Redis: %w", gameID, err)
	}
	if result == 0 {
		return fmt.Errorf("finish game %q in Redis: record does not exist", gameID)
	}
	return nil
}

func (s *Store) Close() error {
	if err := s.client.Close(); err != nil {
		return fmt.Errorf("close Redis client: %w", err)
	}
	return nil
}

func (s *Store) gameKey(id string) string { return s.keyPrefix + "game:" + id }
func (s *Store) activeKey() string        { return s.keyPrefix + "games:active" }
