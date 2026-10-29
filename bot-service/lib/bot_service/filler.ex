defmodule BotService.Filler do
  @moduledoc """
  Casos de uso de la API: llenar una sala con bots, quitarlos y listarlos.

  Solo el anfitrión puede llenar o vaciar. El anfitrión es el jugador de menor
  asiento: quien creó la sala (asiento 0) o, si salió, el que lleva más tiempo.
  La identidad se comprueba preguntándole al Lobby con el token del usuario, así
  que este servicio no necesita conocer `JWT_SECRET`.

  Cada bot se registra en el Lobby como cualquier jugador (sesión + unirse) y
  después se arranca su proceso bajo el supervisor. El cupo lo controla el Lobby
  con su candado por sala: si un humano entra al mismo tiempo, el bot sobrante
  recibe `lobby_full` y simplemente no se agrega.
  """
  alias BotService.{ApiError, Bot, LobbyClient, Names}

  @spec fill(String.t(), String.t(), pos_integer() | nil) ::
          {:ok, map(), [map()]} | {:error, ApiError.t()}
  def fill(lobby_id, token, requested) do
    # Serializa los llenados de una misma sala (p. ej. un doble clic).
    :global.trans({{__MODULE__, lobby_id}, self()}, fn -> do_fill(lobby_id, token, requested) end)
  end

  @spec remove(String.t(), String.t()) :: {:ok, non_neg_integer()} | {:error, ApiError.t()}
  def remove(lobby_id, token) do
    with {:ok, _lobby} <- authorize_host(lobby_id, token) do
      removed =
        lobby_id
        |> Bot.in_lobby()
        |> Enum.count(fn {_id, pid} -> safe_call(fn -> Bot.leave(pid) end) == :ok end)

      {:ok, removed}
    end
  end

  @spec list(String.t(), String.t()) :: {:ok, [map()]} | {:error, ApiError.t()}
  def list(lobby_id, token) do
    with {:ok, lobby} <- client().get_lobby(lobby_id, token),
         {:ok, _me} <- member(lobby, token) do
      bots =
        lobby_id
        |> Bot.in_lobby()
        |> Enum.map(fn {_id, pid} -> safe_call(fn -> Bot.info(pid) end) end)
        |> Enum.reject(&is_nil/1)

      {:ok, bots}
    end
  end

  defp do_fill(lobby_id, token, requested) do
    with {:ok, lobby} <- authorize_host(lobby_id, token),
         {:ok, count} <- seats_to_fill(lobby, requested) do
      taken = Enum.map(lobby["players"], & &1["nickname"])

      bots =
        count
        |> Names.pick(taken)
        |> Enum.reduce_while([], fn name, added ->
          case add_bot(lobby_id, name) do
            {:ok, bot} -> {:cont, [bot | added]}
            {:error, _full_or_unavailable} -> {:halt, added}
          end
        end)
        |> Enum.reverse()

      if bots == [] do
        {:error, ApiError.lobby_full()}
      else
        with {:ok, lobby} <- client().get_lobby(lobby_id, token), do: {:ok, lobby, bots}
      end
    end
  end

  defp authorize_host(lobby_id, token) do
    with {:ok, lobby} <- client().get_lobby(lobby_id, token),
         {:ok, me} <- member(lobby, token) do
      host = Enum.min_by(lobby["players"], & &1["seat"])

      cond do
        lobby["status"] != "WAITING" -> {:error, ApiError.lobby_started()}
        host["id"] != me["id"] -> {:error, ApiError.not_lobby_host()}
        true -> {:ok, lobby}
      end
    end
  end

  defp member(lobby, token) do
    with {:ok, %{"player" => me}} <- client().get_session(token) do
      if Enum.any?(lobby["players"], &(&1["id"] == me["id"])),
        do: {:ok, me},
        else: {:error, ApiError.not_in_lobby()}
    end
  end

  defp seats_to_fill(lobby, requested) do
    free = lobby["maxPlayers"] - lobby["playerCount"]

    if free <= 0,
      do: {:error, ApiError.lobby_full()},
      else: {:ok, min(requested || free, free)}
  end

  defp add_bot(lobby_id, nickname) do
    with {:ok, %{"player" => player, "token" => bot_token}} <- client().create_session(nickname),
         {:ok, _lobby} <- client().join(lobby_id, bot_token),
         {:ok, _pid} <-
           DynamicSupervisor.start_child(
             BotService.BotSupervisor,
             {Bot, %{player: player, token: bot_token, lobby_id: lobby_id}}
           ) do
      {:ok, %{id: player["id"], nickname: player["nickname"]}}
    end
  end

  defp safe_call(fun) do
    fun.()
  catch
    :exit, _bot_already_gone -> nil
  end

  defp client, do: LobbyClient.impl()
end
