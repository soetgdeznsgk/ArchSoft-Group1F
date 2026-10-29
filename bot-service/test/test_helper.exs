Code.require_file("support/fake_lobby.ex", __DIR__)

# Los bots que arrancan las pruebas no deben alcanzar ningún servicio real.
Application.put_env(:bot_service, :lobby_url, "http://127.0.0.1:9")
Application.put_env(:bot_service, :game_logic_url, "http://127.0.0.1:9")

ExUnit.start()
