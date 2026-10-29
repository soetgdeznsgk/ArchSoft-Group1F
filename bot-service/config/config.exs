import Config

config :logger, :default_formatter,
  format: "$time [$level] $message $metadata\n",
  metadata: [:lobby, :bot]

# En pruebas no se abre el puerto HTTP ni se habla con los otros servicios.
config :bot_service, server: config_env() != :test

if config_env() == :test do
  config :logger, level: :warning
  config :bot_service, lobby_client: BotService.FakeLobby
end
