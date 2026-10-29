defmodule BotService.MeldsTest do
  use ExUnit.Case, async: true
  alias BotService.Melds

  # Mismos IDs que el mazo de Go: figura f usa 8*(f-1)..8*(f-1)+7; comodines 56..59.
  defp card(id) when id >= 56, do: %{"id" => id, "figure" => 0, "joker" => true}
  defp card(id), do: %{"id" => id, "figure" => div(id, 8) + 1, "joker" => false}
  defp hand(ids), do: Enum.map(ids, &card/1)

  defp valid?(melds, sizes, ids) do
    used = List.flatten(melds)

    Enum.sort(Enum.map(melds, &length/1)) == Enum.sort(sizes) and
      length(used) == length(Enum.uniq(used)) and
      Enum.all?(used, &(&1 in ids)) and
      Enum.all?(melds, fn meld ->
        figures = meld |> Enum.map(&card/1) |> Enum.reject(& &1["joker"]) |> Enum.map(& &1["figure"])
        figures != [] and length(Enum.uniq(figures)) == 1
      end)
  end

  test "trío con tres cartas de la misma figura" do
    assert Melds.find(hand([0, 1, 2, 9]), [3]) |> Enum.map(&Enum.sort/1) == [[0, 1, 2]]
  end

  test "completa con comodines, pero siempre con al menos una normal" do
    ids = [0, 56, 57, 20]
    melds = Melds.find(hand(ids), [3])
    assert valid?(melds, [3], ids)
    assert Melds.find(hand([56, 57, 58]), [3]) == nil
  end

  test "prefiere la combinación que gasta menos comodines" do
    ids = [0, 1, 2, 8, 56]
    assert Melds.find(hand(ids), [3]) |> List.flatten() |> Enum.sort() == [0, 1, 2]
  end

  test "dos jugadas de la misma figura se reparten las cartas normales" do
    # Tres cartas de la figura 1 y tres comodines: 2 + 1 normales, 1 + 2 comodines.
    ids = [0, 1, 2, 56, 57, 58]
    melds = Melds.find(hand(ids), [3, 3])
    assert valid?(melds, [3, 3], ids)
  end

  test "ronda 5: dos cuartetos" do
    ids = [0, 1, 2, 3, 8, 9, 56, 57]
    assert valid?(Melds.find(hand(ids), [4, 4]), [4, 4], ids)
    assert Melds.find(hand([0, 1, 2, 3, 8, 9, 56, 30]), [4, 4]) == nil
  end

  test "sin jugada posible devuelve nil" do
    assert Melds.find(hand([0, 8, 16]), [3]) == nil
    assert Melds.find([], [3]) == nil
  end

  test "propiedad: toda jugada encontrada es válida según las reglas de Go" do
    for _ <- 1..500 do
      sizes = Enum.random([[3], [4], [3, 3], [4, 3], [4, 4]])
      ids = Enum.take_random(0..59, Enum.random(3..12))

      case Melds.find(hand(ids), sizes) do
        nil -> :ok
        melds -> assert valid?(melds, sizes, ids), "mano #{inspect(ids)} #{inspect(sizes)}: #{inspect(melds)}"
      end
    end
  end
end
