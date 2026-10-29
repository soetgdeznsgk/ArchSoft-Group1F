defmodule BotService.LobbyClient do
  @moduledoc """
  Cliente REST del Lobby Manager. Usa la misma API pública que el navegador
  (docs/lobby-manager.md §2): para el Lobby, un bot es un jugador más.
  Las llamadas van directo por la red interna, sin pasar por Traefik.
  """
  alias BotService.{ApiError, Config}

  @type result :: {:ok, term()} | {:error, ApiError.t()}

  @callback get_session(String.t()) :: result
  @callback get_lobby(String.t(), String.t()) :: result
  @callback create_session(String.t()) :: result
  @callback join(String.t(), String.t()) :: result
  @callback leave(String.t(), String.t()) :: result

  @doc "Módulo en uso; las pruebas lo reemplazan por uno falso."
  def impl, do: Application.get_env(:bot_service, :lobby_client, __MODULE__)

  def get_session(token), do: request(:get, "/api/session", token)
  def get_lobby(lobby_id, token), do: request(:get, "/api/lobbies/#{lobby_id}", token)
  def create_session(nickname), do: request(:post, "/api/session", nil, %{nickname: nickname})
  def join(lobby_id, token), do: request(:post, "/api/lobbies/#{lobby_id}/join", token)
  def leave(lobby_id, token), do: request(:post, "/api/lobbies/#{lobby_id}/leave", token)

  defp request(method, path, token, body \\ nil) do
    opts =
      [method: method, url: path, base_url: Config.lobby_url(), receive_timeout: 5_000, retry: false]
      |> then(&if token, do: Keyword.put(&1, :auth, {:bearer, token}), else: &1)
      |> then(&if body, do: Keyword.put(&1, :json, body), else: &1)

    case Req.request(opts) do
      {:ok, %Req.Response{status: status, body: body}} when status in 200..299 -> {:ok, body}
      {:ok, %Req.Response{status: status, body: body}} -> {:error, ApiError.from_upstream(status, body)}
      {:error, _exception} -> {:error, ApiError.unavailable()}
    end
  end
end
