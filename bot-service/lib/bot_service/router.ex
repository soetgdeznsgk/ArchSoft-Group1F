defmodule BotService.Router do
  @moduledoc """
  API HTTP del servicio. Traefik enruta `/api/bots/` aquí (tiene prioridad sobre
  `/api/`, que va al Lobby). `/healthz` solo se usa dentro de la red interna.

  | Método y ruta                         | Respuesta                      |
  |---------------------------------------|--------------------------------|
  | `POST /api/bots/lobbies/{id}/fill`    | `201 {lobbyId, bots, lobby}`   |
  | `DELETE /api/bots/lobbies/{id}`       | `200 {lobbyId, removed}`       |
  | `GET /api/bots/lobbies/{id}`          | `200 {lobbyId, bots}`          |
  | `GET /healthz`                        | `200 {status, bots}`           |
  """
  use Plug.Router
  use Plug.ErrorHandler

  alias BotService.{ApiError, Filler}

  @max_bots 3

  plug :match
  plug Plug.Parsers, parsers: [:json], json_decoder: Jason, pass: ["*/*"]
  plug :dispatch

  get "/healthz" do
    %{active: bots} = DynamicSupervisor.count_children(BotService.BotSupervisor)
    send_json(conn, 200, %{status: "ok", bots: bots})
  end

  post "/api/bots/lobbies/:lobby_id/fill" do
    with {:ok, token} <- bearer(conn),
         {:ok, lobby_id} <- valid_id(lobby_id),
         {:ok, count} <- requested_count(conn.body_params),
         {:ok, lobby, bots} <- Filler.fill(lobby_id, token, count) do
      send_json(conn, 201, %{lobbyId: lobby_id, bots: bots, lobby: lobby})
    else
      {:error, error} -> send_error(conn, error)
    end
  end

  delete "/api/bots/lobbies/:lobby_id" do
    with {:ok, token} <- bearer(conn),
         {:ok, lobby_id} <- valid_id(lobby_id),
         {:ok, removed} <- Filler.remove(lobby_id, token) do
      send_json(conn, 200, %{lobbyId: lobby_id, removed: removed})
    else
      {:error, error} -> send_error(conn, error)
    end
  end

  get "/api/bots/lobbies/:lobby_id" do
    with {:ok, token} <- bearer(conn),
         {:ok, lobby_id} <- valid_id(lobby_id),
         {:ok, bots} <- Filler.list(lobby_id, token) do
      send_json(conn, 200, %{lobbyId: lobby_id, bots: bots})
    else
      {:error, error} -> send_error(conn, error)
    end
  end

  match _ do
    send_error(conn, ApiError.not_found())
  end

  @impl Plug.ErrorHandler
  def handle_errors(conn, %{reason: %Plug.Parsers.ParseError{}}) do
    send_error(conn, ApiError.invalid_request("El cuerpo no es un JSON válido."))
  end

  def handle_errors(conn, _error) do
    send_error(conn, ApiError.new(500, "internal_error", "Error interno del servicio de bots."))
  end

  defp bearer(conn) do
    case get_req_header(conn, "authorization") do
      ["Bearer " <> token] when token != "" -> {:ok, token}
      _missing -> {:error, ApiError.invalid_session()}
    end
  end

  # El ID termina en una URL del Lobby: solo se aceptan los caracteres que este genera.
  defp valid_id(id) do
    if Regex.match?(~r/\A[A-Za-z0-9_-]{1,64}\z/, id),
      do: {:ok, id},
      else: {:error, ApiError.new(404, "lobby_not_found", "La sala no existe.")}
  end

  defp requested_count(%{"count" => count}) when is_integer(count) and count in 1..@max_bots,
    do: {:ok, count}

  defp requested_count(%{"count" => nil}), do: {:ok, nil}

  defp requested_count(%{"count" => _invalid}),
    do: {:error, ApiError.invalid_request("count debe ser un entero entre 1 y #{@max_bots}.")}

  defp requested_count(_no_count), do: {:ok, nil}

  defp send_error(conn, %ApiError{} = error) do
    send_json(conn, error.status, %{code: error.code, message: error.message})
  end

  defp send_json(conn, status, body) do
    conn
    |> put_resp_content_type("application/json")
    |> send_resp(status, Jason.encode!(body))
  end
end
