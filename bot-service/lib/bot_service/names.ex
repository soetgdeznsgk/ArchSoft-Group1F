defmodule BotService.Names do
  @moduledoc """
  Nombres de los bots. Todos empiezan con "Bot " para que la sala y la mesa los
  distingan de los humanos sin cambiar el contrato del Lobby, y caben en el límite
  de 20 caracteres del nickname.
  """

  @names ~w(Cometa Trébol Lince Pixel Tango Nova Kiwi Rayo Eco Brisa Zafiro Quásar)

  @spec pick(non_neg_integer(), [String.t()]) :: [String.t()]
  def pick(count, taken) do
    taken = MapSet.new(taken)

    @names
    |> Enum.map(&("Bot " <> &1))
    |> Enum.reject(&MapSet.member?(taken, &1))
    |> Enum.shuffle()
    |> Enum.take(count)
  end
end
