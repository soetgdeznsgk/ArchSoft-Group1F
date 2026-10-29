defmodule BotService.ApiError do
  @moduledoc """
  Error de la API con la misma forma que usa el Lobby Manager:
  `{"code": "...", "message": "..."}`, con `message` en español para mostrarlo tal cual.
  """
  defexception [:status, :code, :message]

  @type t :: %__MODULE__{status: pos_integer(), code: String.t(), message: String.t()}

  def new(status, code, message), do: %__MODULE__{status: status, code: code, message: message}

  @doc "Respuesta de error del Lobby Manager. Los 4xx se reenvían tal cual; los 5xx pasan a 502."
  def from_upstream(status, %{"code" => code, "message" => message}) when status in 400..499,
    do: new(status, code, message)

  def from_upstream(_status, _body), do: unavailable()

  def unavailable,
    do: new(502, "lobby_service_unavailable", "El servicio de salas no respondió. Intenta de nuevo.")

  def invalid_session,
    do: new(401, "invalid_session", "Tu sesión no es válida. Vuelve a ingresar tu nombre.")

  def invalid_request(message), do: new(422, "invalid_request", message)
  def not_found, do: new(404, "not_found", "Ruta no encontrada.")
  def not_in_lobby, do: new(403, "not_in_lobby", "No perteneces a esta sala.")

  def not_lobby_host,
    do: new(403, "not_lobby_host", "Solo el anfitrión de la sala puede agregar o quitar bots.")

  def lobby_started, do: new(409, "lobby_started", "La partida ya inició.")
  def lobby_full, do: new(409, "lobby_full", "La sala ya está llena.")
end
