defmodule BotService.FakeLobby do
  @moduledoc "Lobby Manager en memoria con las mismas respuestas que el real, para las pruebas."
  @behaviour BotService.LobbyClient

  alias BotService.ApiError

  @lobby_id "l_test"

  def start(players, opts \\ []) do
    sessions =
      Map.new(players, fn {token, id, nickname} -> {token, %{"id" => id, "nickname" => nickname}} end)

    lobby = %{
      "id" => @lobby_id,
      "code" => "ABC234",
      "status" => Keyword.get(opts, :status, "WAITING"),
      "players" =>
        players
        |> Enum.with_index()
        |> Enum.map(fn {{_token, id, nickname}, seat} ->
          %{"id" => id, "nickname" => nickname, "seat" => seat}
        end),
      "maxPlayers" => 4,
      "game" => nil
    }

    Agent.start_link(fn -> %{sessions: sessions, lobby: count(lobby), next: 0} end, name: __MODULE__)
  end

  def lobby, do: Agent.get(__MODULE__, & &1.lobby)

  @impl true
  def get_session(token) do
    case Agent.get(__MODULE__, &Map.get(&1.sessions, token)) do
      nil -> {:error, ApiError.invalid_session()}
      player -> {:ok, %{"player" => player, "lobby" => nil}}
    end
  end

  @impl true
  def get_lobby(@lobby_id, token) do
    with {:ok, _player} <- get_session(token), do: {:ok, lobby()}
  end

  def get_lobby(_other, _token), do: {:error, ApiError.new(404, "lobby_not_found", "La sala no existe.")}

  @impl true
  def create_session(nickname) do
    Agent.get_and_update(__MODULE__, fn state ->
      id = "p_bot#{state.next}"
      token = "t_bot#{state.next}"
      player = %{"id" => id, "nickname" => nickname}
      state = %{state | next: state.next + 1, sessions: Map.put(state.sessions, token, player)}
      {{:ok, %{"player" => player, "token" => token}}, state}
    end)
  end

  @impl true
  def join(@lobby_id, token) do
    Agent.get_and_update(__MODULE__, fn %{lobby: lobby} = state ->
      player = state.sessions[token]

      if lobby["playerCount"] >= lobby["maxPlayers"] do
        {{:error, ApiError.lobby_full()}, state}
      else
        seat = lobby["players"] |> Enum.map(& &1["seat"]) |> Enum.max(fn -> -1 end)
        entry = Map.put(player, "seat", seat + 1)
        lobby = count(%{lobby | "players" => lobby["players"] ++ [entry]})
        {{:ok, lobby}, %{state | lobby: lobby}}
      end
    end)
  end

  @impl true
  def leave(@lobby_id, token) do
    Agent.get_and_update(__MODULE__, fn %{lobby: lobby} = state ->
      id = state.sessions[token]["id"]
      lobby = count(%{lobby | "players" => Enum.reject(lobby["players"], &(&1["id"] == id))})
      {{:ok, ""}, %{state | lobby: lobby}}
    end)
  end

  defp count(lobby), do: Map.put(lobby, "playerCount", length(lobby["players"]))
end
