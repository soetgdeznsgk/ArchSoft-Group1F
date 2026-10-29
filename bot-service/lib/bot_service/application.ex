defmodule BotService.Application do
  @moduledoc false
  use Application

  @impl true
  def start(_type, _args) do
    children =
      [
        # player_id del bot -> lobby_id. Sirve para encontrar los bots de una sala.
        {Registry, keys: :unique, name: BotService.Registry},
        # Un proceso por bot. Si uno falla, se reinicia solo sin afectar a los demás.
        {DynamicSupervisor,
         name: BotService.BotSupervisor, strategy: :one_for_one, max_restarts: 100, max_seconds: 10}
      ] ++ server()

    Supervisor.start_link(children, strategy: :one_for_one, name: BotService.Supervisor)
  end

  defp server do
    if Application.get_env(:bot_service, :server) do
      [{Bandit, plug: BotService.Router, port: BotService.Config.port()}]
    else
      []
    end
  end
end
