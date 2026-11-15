defmodule BotService.AuthClient do
  @moduledoc """
  Cliente REST de Auth. Cada bot pide su sesión de invitado igual que el menú
  del navegador (`POST /auth/guest`): Auth es quien emite las sesiones.
  Las llamadas van directo por la red interna, sin pasar por Traefik.
  """
  alias BotService.{ApiError, Config}

  @callback guest_session(String.t()) :: {:ok, term()} | {:error, ApiError.t()}

  @doc "Módulo en uso; las pruebas lo reemplazan por uno falso."
  def impl, do: Application.get_env(:bot_service, :auth_client, __MODULE__)

  def guest_session(nickname) do
    opts = [
      method: :post,
      url: "/auth/guest",
      base_url: Config.auth_url(),
      json: %{nickname: nickname},
      receive_timeout: 5_000,
      retry: false
    ]

    case Req.request(opts) do
      {:ok, %Req.Response{status: status, body: body}} when status in 200..299 -> {:ok, body}
      {:ok, %Req.Response{status: status, body: body}} when status in 400..499 -> rejected(status, body)
      _failure -> {:error, ApiError.auth_unavailable()}
    end
  end

  defp rejected(status, body), do: {:error, ApiError.from_upstream(status, body)}
end
