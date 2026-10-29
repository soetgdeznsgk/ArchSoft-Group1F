defmodule BotService.LobbyStream do
  @moduledoc """
  Conexión SSE a la sala de espera (`GET /api/lobbies/{id}/events`).

  Usa Mint en modo activo: los datos del socket llegan como mensajes al proceso
  que abrió la conexión (el bot) y este los pasa a `handle/2`. Mantenerla abierta
  es lo que evita que el Lobby saque al bot por ausencia (30 s sin SSE).
  """
  alias BotService.SSE

  defstruct [:conn, :ref, buffer: ""]

  @type t :: %__MODULE__{}

  @spec open(String.t(), String.t(), String.t()) :: {:ok, t()} | {:error, term()}
  def open(base_url, lobby_id, token) do
    uri = URI.parse(base_url)
    path = "/api/lobbies/#{lobby_id}/events?" <> URI.encode_query(token: token)

    with {:ok, conn} <- Mint.HTTP.connect(scheme(uri), uri.host, uri.port, protocols: [:http1]),
         {:ok, conn, ref} <- Mint.HTTP.request(conn, "GET", path, [{"accept", "text/event-stream"}], nil) do
      {:ok, %__MODULE__{conn: conn, ref: ref}}
    else
      {:error, reason} -> {:error, reason}
      {:error, conn, reason} -> close_conn(conn, {:error, reason})
    end
  end

  @doc """
  Procesa un mensaje del proceso. Devuelve `:unknown` si no es de esta conexión,
  `{:ok, stream, eventos}` o `{:closed, motivo, eventos}` (los eventos que llegaron
  junto con el cierre, p. ej. `lobby_closed`).
  """
  @spec handle(t(), term()) ::
          :unknown | {:ok, t(), [{String.t(), String.t()}]} | {:closed, term(), [{String.t(), String.t()}]}
  def handle(%__MODULE__{conn: conn} = stream, message) do
    case Mint.HTTP.stream(conn, message) do
      :unknown -> :unknown
      {:ok, conn, responses} -> consume(%{stream | conn: conn}, responses, [])
      {:error, conn, reason, _responses} -> close_conn(conn, {:closed, reason, []})
    end
  end

  def close(%__MODULE__{conn: conn}), do: Mint.HTTP.close(conn)
  def close(nil), do: :ok

  defp consume(stream, [], events), do: {:ok, stream, events}

  defp consume(%{ref: ref} = stream, [response | rest], events) do
    case response do
      {:status, ^ref, 200} ->
        consume(stream, rest, events)

      {:status, ^ref, status} ->
        close_conn(stream.conn, {:closed, {:http_status, status}, events})

      {:data, ^ref, chunk} ->
        {new_events, buffer} = SSE.parse(stream.buffer <> chunk)
        consume(%{stream | buffer: buffer}, rest, events ++ new_events)

      {:done, ^ref} ->
        close_conn(stream.conn, {:closed, :done, events})

      {:error, ^ref, reason} ->
        close_conn(stream.conn, {:closed, reason, events})

      _headers ->
        consume(stream, rest, events)
    end
  end

  defp close_conn(conn, result) do
    Mint.HTTP.close(conn)
    result
  end

  defp scheme(%URI{scheme: "https"}), do: :https
  defp scheme(_uri), do: :http
end
