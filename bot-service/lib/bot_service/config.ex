defmodule BotService.Config do
  @moduledoc "Lectura de la configuración de config/runtime.exs."

  def port, do: get(:port)
  def lobby_url, do: get(:lobby_url)
  def game_logic_url, do: get(:game_logic_url)
  def think_ms, do: get(:think_ms)
  def react_ms, do: get(:react_ms)
  def lobby_timeout_ms, do: get(:lobby_timeout_ms)

  defp get(key), do: Application.fetch_env!(:bot_service, key)
end
