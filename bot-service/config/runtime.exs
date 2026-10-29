import Config

# Toda la configuración llega por variables de entorno (docker-compose.yml).
int = fn name, default -> name |> System.get_env(to_string(default)) |> String.to_integer() end

config :bot_service,
  port: int.("PORT", 8090),
  lobby_url: System.get_env("LOBBY_URL", "http://lobby-manager:8000"),
  game_logic_url: System.get_env("GAME_LOGIC_URL", "http://game-logic:8081"),
  # Pausa antes de cada acción en su turno, para que los humanos alcancen a ver la jugada.
  think_ms: {int.("BOT_THINK_MIN_MS", 900), int.("BOT_THINK_MAX_MS", 2200)},
  # Tiempo de reacción para robar en el turno de otro.
  react_ms: {int.("BOT_REACT_MIN_MS", 400), int.("BOT_REACT_MAX_MS", 1200)},
  # Si la partida no inicia en este tiempo, los bots salen de la sala y liberan el cupo.
  lobby_timeout_ms: int.("BOT_LOBBY_TIMEOUT_S", 600) * 1000
