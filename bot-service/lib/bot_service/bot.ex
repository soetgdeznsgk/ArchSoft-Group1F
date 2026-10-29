defmodule BotService.Bot do
  @moduledoc """
  Un bot es un proceso que se comporta como un jugador más, con el mismo
  recorrido que el navegador (docs/lobby-manager.md §1):

      :waiting  --game_starting-->  :playing  --game_ended-->  fin
         |                              |
       SSE de la sala              WebSocket de la partida

  - En `:waiting` mantiene abierto el SSE de la sala. Si en la sala ya no queda
    ningún humano, o la partida no inicia a tiempo, sale y libera el cupo.
  - En `:playing` recibe la vista (`view`) tras cada cambio y responde con
    acciones, con una pausa de "pensar" para que los humanos vean la jugada.

  Las caídas de red se reintentan con espera creciente. Si el proceso falla, el
  supervisor lo reinicia y el bot retoma desde el estado de la sala (`sync`):
  vuelve a pedir su acceso a la partida y se reconecta. Go mientras tanto juega
  por él en automático (RF-20), igual que con un humano desconectado.
  """
  use GenServer, restart: :transient
  require Logger

  alias BotService.{Config, GameSocket, LobbyClient, LobbyStream, Strategy}

  @max_retries 8
  @awaiting_timeout_ms 5_000

  # --- API --------------------------------------------------------------------

  def start_link(%{player: %{"id" => id}, lobby_id: lobby_id} = args) do
    GenServer.start_link(__MODULE__, args, name: {:via, Registry, {BotService.Registry, id, lobby_id}})
  end

  @doc "Saca al bot de la sala. Solo funciona mientras la partida no ha iniciado."
  def leave(pid), do: GenServer.call(pid, :leave, 10_000)

  def info(pid), do: GenServer.call(pid, :info)

  @doc "IDs de jugador de todos los bots vivos de una sala."
  def in_lobby(lobby_id) do
    Registry.select(BotService.Registry, [
      {{:"$1", :"$2", :"$3"}, [{:==, :"$3", lobby_id}], [{{:"$1", :"$2"}}]}
    ])
  end

  # --- ciclo de vida ----------------------------------------------------------

  @impl true
  def init(%{player: player, token: token, lobby_id: lobby_id}) do
    Logger.metadata(lobby: lobby_id, bot: player["nickname"])
    Process.send_after(self(), :lobby_timeout, Config.lobby_timeout_ms())

    state = %{
      player: player,
      token: token,
      lobby_id: lobby_id,
      stage: :waiting,
      stream: nil,
      socket: nil,
      game: nil,
      view: nil,
      timer: nil,
      awaiting: nil,
      blocked: MapSet.new(),
      steal_key: nil,
      steal_wanted: nil,
      retries: 0,
      personality: Strategy.personality()
    }

    {:ok, state, {:continue, :sync}}
  end

  @impl true
  def handle_continue(:sync, state), do: sync(state)

  @impl true
  def handle_call(:info, _from, state) do
    {:reply, %{id: state.player["id"], nickname: state.player["nickname"], stage: state.stage}, state}
  end

  def handle_call(:leave, _from, %{stage: :waiting} = state) do
    LobbyStream.close(state.stream)
    _ = LobbyClient.impl().leave(state.lobby_id, state.token)
    {:stop, :normal, :ok, %{state | stream: nil}}
  end

  def handle_call(:leave, _from, state), do: {:reply, {:error, :playing}, state}

  @impl true
  def handle_info({:retry, step}, state), do: run(step, state)

  def handle_info(:lobby_timeout, %{stage: :waiting} = state) do
    Logger.info("La partida no inició a tiempo; el bot sale de la sala")
    leave_and_stop(state)
  end

  def handle_info(:lobby_timeout, state), do: {:noreply, state}

  def handle_info(:act, state), do: act(%{state | timer: nil})

  def handle_info({:awaiting_timeout, action}, %{awaiting: action} = state) do
    schedule(%{state | awaiting: nil})
  end

  def handle_info({:awaiting_timeout, _stale}, state), do: {:noreply, state}

  def handle_info(message, state) do
    with :unknown <- stream_message(state, message),
         :unknown <- socket_message(state, message) do
      # Mensajes tardíos de conexiones que ya se cerraron.
      {:noreply, state}
    end
  end

  @impl true
  def terminate(_reason, state) do
    LobbyStream.close(state.stream)
    GameSocket.close(state.socket)
  end

  # --- sala de espera -----------------------------------------------------------

  defp run(:sync, state), do: sync(state)
  defp run(:stream, state), do: open_stream(state)
  defp run(:game, state), do: connect_game(state, state.game)

  # Lee la sala con el token del bot. Es el punto de partida al iniciar y al reiniciarse.
  defp sync(state) do
    case LobbyClient.impl().get_lobby(state.lobby_id, state.token) do
      {:ok, lobby} ->
        cond do
          not member?(lobby, state) -> {:stop, {:shutdown, :not_in_lobby}, state}
          lobby["status"] == "CLOSED" -> {:stop, :normal, state}
          lobby["game"] != nil -> connect_game(state, lobby["game"])
          true -> open_stream(state)
        end

      {:error, %{status: status}} when status in [401, 403, 404] ->
        {:stop, {:shutdown, :lobby_gone}, state}

      {:error, _unavailable} ->
        retry(state, :sync)
    end
  end

  defp open_stream(state) do
    case LobbyStream.open(Config.lobby_url(), state.lobby_id, state.token) do
      {:ok, stream} -> {:noreply, %{state | stream: stream}}
      {:error, _reason} -> retry(state, :stream)
    end
  end

  defp stream_message(%{stream: nil}, _message), do: :unknown

  defp stream_message(state, message) do
    case LobbyStream.handle(state.stream, message) do
      :unknown ->
        :unknown

      {:ok, stream, events} ->
        lobby_events(events, %{state | stream: stream, retries: 0})

      {:closed, reason, events} ->
        with {:noreply, state} <- lobby_events(events, %{state | stream: nil}) do
          case reason do
            {:http_status, status} when status in 400..499 -> {:stop, {:shutdown, :not_in_lobby}, state}
            _closed when state.stage == :waiting -> retry(state, :stream)
            _closed -> {:noreply, state}
          end
        end
    end
  end

  defp lobby_events(events, state) do
    Enum.reduce_while(events, {:noreply, state}, fn event, {:noreply, state} ->
      case lobby_event(event, state) do
        {:noreply, _state} = next -> {:cont, next}
        stop -> {:halt, stop}
      end
    end)
  end

  defp lobby_event({"lobby", data}, %{stage: :waiting} = state) do
    lobby = Jason.decode!(data)
    humans = Enum.reject(lobby["players"], &bot?/1)

    cond do
      not member?(lobby, state) ->
        {:stop, {:shutdown, :not_in_lobby}, state}

      lobby["status"] == "WAITING" and humans == [] ->
        Logger.info("No quedan humanos en la sala; el bot sale")
        leave_and_stop(state)

      true ->
        {:noreply, state}
    end
  end

  defp lobby_event({"game_starting", data}, %{stage: :waiting} = state) do
    LobbyStream.close(state.stream)
    connect_game(%{state | stream: nil}, Jason.decode!(data))
  end

  defp lobby_event({"lobby_closed", _data}, state), do: {:stop, :normal, state}
  defp lobby_event(_event, state), do: {:noreply, state}

  defp leave_and_stop(state) do
    LobbyStream.close(state.stream)
    _ = LobbyClient.impl().leave(state.lobby_id, state.token)
    {:stop, :normal, %{state | stream: nil}}
  end

  # --- partida ------------------------------------------------------------------

  defp connect_game(state, game) do
    state = %{state | stage: :playing, game: game}

    case GameSocket.open(Config.game_logic_url(), game["wsPath"], game["token"]) do
      {:ok, socket} ->
        Logger.info("Conectado a la partida #{game["gameId"]}")
        {:noreply, %{state | socket: socket, retries: 0}}

      {:error, _reason} ->
        retry(state, :game)
    end
  end

  defp socket_message(%{socket: nil}, _message), do: :unknown

  defp socket_message(state, message) do
    case GameSocket.handle(state.socket, message) do
      :unknown ->
        :unknown

      {:ok, socket, messages} ->
        state = Enum.reduce(messages, %{state | socket: socket}, &game_message/2)
        if state.stage == :finished, do: finish(state), else: schedule(state)

      {:closed, reason, messages} ->
        state = Enum.reduce(messages, %{state | socket: nil}, &game_message/2)

        case reason do
          _any when state.stage == :finished -> finish(state)
          # El token de partida venció: la sala entrega uno nuevo.
          {:upgrade_failed, 401} -> retry(state, :sync)
          {:upgrade_failed, 410} -> {:stop, :normal, state}
          {:upgrade_failed, status} -> {:stop, {:shutdown, {:rejected, status}}, state}
          _network -> retry(state, :game)
        end
    end
  end

  defp game_message(%{"type" => "view", "view" => view}, state) do
    state = %{state | view: view, awaiting: nil}

    cond do
      view["phase"] == "game_over" -> %{state | stage: :finished}
      view["current_turn"] != view["you"] -> %{state | blocked: MapSet.new()}
      true -> state
    end
  end

  defp game_message(%{"type" => "error", "code" => code} = error, %{awaiting: action} = state)
       when action != nil do
    Logger.debug("Go rechazó #{action["type"]}: #{code} #{error["message"]}")
    %{state | awaiting: nil, blocked: MapSet.put(state.blocked, Strategy.blocked_by(action))}
  end

  defp game_message(%{"type" => "game_ended"}, state), do: %{state | stage: :finished}
  defp game_message(_event, state), do: state

  defp finish(state) do
    Logger.info("Partida terminada")
    GameSocket.close(state.socket)
    {:stop, :normal, %{state | socket: nil}}
  end

  # Programa la próxima acción según la vista más reciente. La acción concreta se
  # calcula al vencer la pausa (en `act/1`), con la vista de ese momento, así que
  # un temporizador viejo nunca envía una acción que ya no aplica.
  defp schedule(%{view: view} = state) when view != nil and state.awaiting == nil do
    cond do
      Strategy.my_turn?(view) ->
        {:noreply, state |> cancel_timer() |> arm(Config.think_ms())}

      # Se decide una sola vez por carta botada.
      view["can_steal"] == true and Strategy.steal_key(view) != state.steal_key ->
        key = Strategy.steal_key(view)

        if Strategy.steal?(view, state.personality, :rand.uniform()),
          do:
            {:noreply,
             state
             |> cancel_timer()
             |> Map.merge(%{steal_key: key, steal_wanted: key})
             |> arm(Config.react_ms())},
          else: {:noreply, %{state | steal_key: key}}

      true ->
        {:noreply, state}
    end
  end

  defp schedule(state), do: {:noreply, state}

  defp act(%{view: view} = state) when view != nil and state.awaiting == nil do
    action =
      cond do
        Strategy.my_turn?(view) ->
          Strategy.turn_action(view, state.blocked)

        view["can_steal"] == true and state.steal_wanted == Strategy.steal_key(view) ->
          %{"type" => "steal"}

        true ->
          nil
      end

    if action, do: send_action(state, action), else: {:noreply, state}
  end

  defp act(state), do: {:noreply, state}

  defp send_action(state, action) do
    case GameSocket.send_json(state.socket, action) do
      {:ok, socket} ->
        Process.send_after(self(), {:awaiting_timeout, action}, @awaiting_timeout_ms)
        {:noreply, %{state | socket: socket, awaiting: action, steal_wanted: nil}}

      {:error, _reason} ->
        GameSocket.close(state.socket)
        retry(%{state | socket: nil}, :game)
    end
  end

  defp arm(state, {min_ms, max_ms}) do
    delay = round((min_ms + :rand.uniform(max(max_ms - min_ms, 1))) * state.personality.pace)
    %{state | timer: Process.send_after(self(), :act, delay)}
  end

  defp cancel_timer(%{timer: nil} = state), do: state

  defp cancel_timer(%{timer: timer} = state) do
    Process.cancel_timer(timer)
    %{state | timer: nil}
  end

  # --- utilidades ---------------------------------------------------------------

  defp retry(state, _step) when state.retries >= @max_retries do
    Logger.warning("Sin conexión tras #{@max_retries} intentos; el bot se detiene")
    {:stop, {:shutdown, :unreachable}, state}
  end

  defp retry(state, step) do
    Process.send_after(self(), {:retry, step}, min(500 * 2 ** state.retries, 15_000))
    {:noreply, %{state | retries: state.retries + 1}}
  end

  defp member?(lobby, state), do: Enum.any?(lobby["players"], &(&1["id"] == state.player["id"]))

  defp bot?(player), do: Registry.lookup(BotService.Registry, player["id"]) != []
end
