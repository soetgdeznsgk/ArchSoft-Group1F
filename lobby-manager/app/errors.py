"""Errores de la API: código estable para el cliente y mensaje informativo en español."""

from __future__ import annotations


class ApiError(Exception):
    def __init__(self, status: int, code: str, message: str) -> None:
        super().__init__(message)
        self.status = status
        self.code = code
        self.message = message


def invalid_nickname(message: str) -> ApiError:
    return ApiError(422, "invalid_nickname", message)


def invalid_session() -> ApiError:
    return ApiError(401, "invalid_session", "Tu sesión no es válida o expiró. Ingresa de nuevo con tu nombre.")


def unauthorized_service() -> ApiError:
    return ApiError(401, "unauthorized", "Token de servicio ausente o incorrecto.")


def lobby_not_found(by_code: bool = False) -> ApiError:
    message = "No existe una sala con ese código." if by_code else "La sala no existe o ya fue cerrada."
    return ApiError(404, "lobby_not_found", message)


def lobby_limit_reached(limit: int) -> ApiError:
    return ApiError(409, "lobby_limit_reached", f"Ya hay {limit} salas activas. Únete a una o inténtalo más tarde.")


def lobby_full() -> ApiError:
    return ApiError(409, "lobby_full", "La sala está llena.")


def lobby_started() -> ApiError:
    return ApiError(409, "lobby_started", "La partida de esta sala ya inició.")


def already_in_lobby() -> ApiError:
    return ApiError(409, "already_in_lobby", "Ya estás en otra sala. Sal de ella antes de entrar a una nueva.")


def not_in_lobby() -> ApiError:
    return ApiError(403, "not_in_lobby", "No perteneces a esta sala.")


def not_enough_players(missing: int) -> ApiError:
    if missing == 1:
        return ApiError(409, "not_enough_players", "Falta 1 jugador para iniciar la partida.")
    return ApiError(409, "not_enough_players", f"Faltan {missing} jugadores para iniciar la partida.")


def game_service_unavailable() -> ApiError:
    return ApiError(502, "game_service_unavailable", "No se pudo iniciar la partida. Inténtalo de nuevo.")


def game_mismatch() -> ApiError:
    return ApiError(409, "game_mismatch", "El gameId no corresponde a la partida de esta sala.")


def invalid_result(message: str) -> ApiError:
    return ApiError(422, "invalid_result", message)
