defmodule BotService.Melds do
  @moduledoc """
  Busca en la mano las jugadas objetivo de la ronda (tríos y cuartetos).

  Las reglas son las de `game-logic` (`ValidateMeld`): tamaño exacto, al menos una
  carta normal y todas las normales de la misma figura; el resto se completa con
  comodines. Dos jugadas pueden ser de la misma figura.

  A diferencia del bot de simulación de Go (voraz), aquí se prueban todas las
  asignaciones de figura a jugada (a lo sumo 7 × 7) y se elige la que gasta menos
  comodines, que conviene guardar para depositar después.
  """

  @doc """
  `hand` son las cartas de la vista (`%{"id", "figure", "joker"}`) y `sizes` los
  tamaños de las jugadas de la ronda (`spec.melds`, p. ej. `[4, 3]`).
  Devuelve la lista de jugadas como IDs de carta, o `nil` si no se pueden formar.
  """
  @spec find([map()], [3 | 4]) :: [[integer()]] | nil
  def find(hand, sizes) do
    {jokers, normals} = Enum.split_with(hand, & &1["joker"])
    by_figure = Enum.group_by(normals, & &1["figure"], & &1["id"])
    jokers = Enum.map(jokers, & &1["id"])

    by_figure
    |> Map.keys()
    |> assignments(length(sizes))
    |> Enum.map(&build(Enum.zip(&1, sizes), by_figure, jokers))
    |> Enum.reject(&is_nil/1)
    |> Enum.min_by(fn {_melds, jokers_used} -> jokers_used end, fn -> nil end)
    |> case do
      nil -> nil
      {melds, _jokers_used} -> melds
    end
  end

  defp assignments(_figures, 0), do: [[]]

  defp assignments(figures, n) do
    for figure <- figures, rest <- assignments(figures, n - 1), do: [figure | rest]
  end

  # Arma las jugadas para una asignación. Si dos jugadas comparten figura, la
  # primera deja al menos una carta normal para cada una de las siguientes.
  defp build(assignment, by_figure, jokers) do
    assignment
    |> Enum.with_index()
    |> Enum.reduce_while({[], by_figure, jokers, 0}, fn {{figure, size}, i}, {melds, left, jokers, used} ->
      available = Map.get(left, figure, [])
      later = assignment |> Enum.drop(i + 1) |> Enum.count(fn {f, _} -> f == figure end)
      take = min(length(available) - later, size)
      need = size - take

      if take < 1 or need > length(jokers) do
        {:halt, nil}
      else
        {normals, remaining} = Enum.split(available, take)
        {wild, jokers_left} = Enum.split(jokers, need)
        {:cont, {[normals ++ wild | melds], Map.put(left, figure, remaining), jokers_left, used + need}}
      end
    end)
    |> case do
      nil -> nil
      {melds, _left, _jokers, used} -> {Enum.reverse(melds), used}
    end
  end
end
