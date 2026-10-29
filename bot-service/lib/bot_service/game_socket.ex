defmodule BotService.GameSocket do
  @moduledoc """
  WebSocket de la partida (`/ws/games/{gameId}?token=`), el mismo que abre el navegador.

  Igual que `LobbyStream`, la conexión es de Mint en modo activo y pertenece al
  proceso del bot. Responde los ping de Go y decodifica cada mensaje de texto como JSON.
  """

  defstruct [:conn, :ref, :websocket, status: nil, headers: []]

  @type t :: %__MODULE__{}

  @spec open(String.t(), String.t(), String.t()) :: {:ok, t()} | {:error, term()}
  def open(base_url, ws_path, token) do
    uri = URI.parse(base_url)
    {http, ws} = if uri.scheme == "https", do: {:https, :wss}, else: {:http, :ws}
    path = ws_path <> "?" <> URI.encode_query(token: token)

    with {:ok, conn} <- Mint.HTTP.connect(http, uri.host, uri.port, protocols: [:http1]),
         {:ok, conn, ref} <- Mint.WebSocket.upgrade(ws, conn, path, []) do
      {:ok, %__MODULE__{conn: conn, ref: ref}}
    else
      {:error, reason} -> {:error, reason}
      {:error, conn, reason} -> close_conn(conn, {:error, reason})
    end
  end

  @doc """
  Procesa un mensaje del proceso. Devuelve `:unknown` si no es de esta conexión,
  `{:ok, socket, mensajes}` o `{:closed, motivo, mensajes}`.

  Si Go rechaza la conexión, el motivo es `{:upgrade_failed, status}` (401, 403, 404, 410).
  """
  @spec handle(t(), term()) :: :unknown | {:ok, t(), [map()]} | {:closed, term(), [map()]}
  def handle(%__MODULE__{conn: conn} = socket, message) do
    case Mint.WebSocket.stream(conn, message) do
      :unknown -> :unknown
      {:ok, conn, responses} -> consume(%{socket | conn: conn}, responses, [])
      {:error, conn, reason, _responses} -> close_conn(conn, {:closed, reason, []})
    end
  end

  @spec send_json(t(), map()) :: {:ok, t()} | {:error, term()}
  def send_json(socket, payload), do: send_frame(socket, {:text, Jason.encode!(payload)})

  def close(%__MODULE__{websocket: nil, conn: conn}), do: Mint.HTTP.close(conn)

  def close(%__MODULE__{} = socket) do
    _ = send_frame(socket, {:close, 1000, "bot done"})
    Mint.HTTP.close(socket.conn)
  end

  def close(nil), do: :ok

  defp consume(socket, [], messages), do: {:ok, socket, messages}

  defp consume(%{ref: ref} = socket, [response | rest], messages) do
    case response do
      {:status, ^ref, status} ->
        consume(%{socket | status: status}, rest, messages)

      {:headers, ^ref, headers} ->
        consume(%{socket | headers: headers}, rest, messages)

      {:done, ^ref} when socket.websocket == nil ->
        case Mint.WebSocket.new(socket.conn, ref, socket.status, socket.headers) do
          {:ok, conn, websocket} -> consume(%{socket | conn: conn, websocket: websocket}, rest, messages)
          {:error, conn, _reason} -> close_conn(conn, {:closed, {:upgrade_failed, socket.status}, messages})
        end

      {:data, ^ref, data} when socket.websocket != nil ->
        case Mint.WebSocket.decode(socket.websocket, data) do
          {:ok, websocket, frames} -> frames(%{socket | websocket: websocket}, frames, rest, messages)
          {:error, _websocket, reason} -> close_conn(socket.conn, {:closed, reason, messages})
        end

      {:error, ^ref, reason} ->
        close_conn(socket.conn, {:closed, reason, messages})

      _other ->
        consume(socket, rest, messages)
    end
  end

  defp frames(socket, [], rest, messages), do: consume(socket, rest, messages)

  defp frames(socket, [frame | frames], rest, messages) do
    case frame do
      {:text, text} ->
        case Jason.decode(text) do
          {:ok, %{} = message} -> frames(socket, frames, rest, messages ++ [message])
          _invalid -> frames(socket, frames, rest, messages)
        end

      {:ping, data} ->
        case send_frame(socket, {:pong, data}) do
          {:ok, socket} -> frames(socket, frames, rest, messages)
          {:error, reason} -> close_conn(socket.conn, {:closed, reason, messages})
        end

      {:close, code, reason} ->
        close_conn(socket.conn, {:closed, {:remote_close, code, reason}, messages})

      _other ->
        frames(socket, frames, rest, messages)
    end
  end

  defp send_frame(%{websocket: websocket, conn: conn, ref: ref} = socket, frame) do
    with {:ok, websocket, data} <- Mint.WebSocket.encode(websocket, frame),
         {:ok, conn} <- Mint.WebSocket.stream_request_body(conn, ref, data) do
      {:ok, %{socket | websocket: websocket, conn: conn}}
    else
      {:error, _websocket_or_conn, reason} -> {:error, reason}
    end
  end

  defp close_conn(conn, result) do
    Mint.HTTP.close(conn)
    result
  end
end
