defmodule BotService.MixProject do
  use Mix.Project

  def project do
    [
      app: :bot_service,
      version: "1.0.0",
      elixir: "~> 1.17",
      start_permanent: Mix.env() == :prod,
      deps: deps(),
      releases: [bot_service: [include_executables_for: [:unix]]]
    ]
  end

  def application do
    [
      extra_applications: [:logger],
      mod: {BotService.Application, []}
    ]
  end

  defp deps do
    [
      {:bandit, "~> 1.6"},
      {:plug, "~> 1.16"},
      {:req, "~> 0.5"},
      {:mint, "~> 1.6"},
      {:mint_web_socket, "~> 1.0"},
      {:jason, "~> 1.4"}
    ]
  end
end
