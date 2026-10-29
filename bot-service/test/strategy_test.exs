defmodule BotService.StrategyTest do
  use ExUnit.Case, async: true
  alias BotService.Strategy

  defp card(id) when id >= 56, do: %{"id" => id, "figure" => 0, "joker" => true}
  defp card(id), do: %{"id" => id, "figure" => div(id, 8) + 1, "joker" => false}

  # Vista mínima con la forma de PlayerView (game-logic/internal/game/view.go).
  defp view(overrides) do
    Map.merge(
      %{
        "you" => "bot",
        "round" => 1,
        "spec" => %{"number" => 1, "cards_per_player" => 3, "melds" => [3]},
        "phase" => "awaiting_draw",
        "current_turn" => "bot",
        "hand" => [],
        "laid_down" => false,
        "top_discard" => nil,
        "discard_available" => false,
        "discard_count" => 0,
        "hidden_count" => 40,
        "banks" => [],
        "can_steal" => false,
        "can_take_discard" => false,
        "can_lay_down" => false,
        "can_deposit" => false,
        "can_discard" => false
      },
      overrides
    )
  end

  defp hand(ids), do: Enum.map(ids, &card/1)
  defp none, do: MapSet.new()

  describe "turno propio" do
    test "baja cuando puede y tiene la jugada" do
      v = view(%{"hand" => hand([0, 1, 2, 20]), "can_lay_down" => true})
      assert %{"type" => "lay_down", "melds" => [meld]} = Strategy.turn_action(v, none())
      assert Enum.sort(meld) == [0, 1, 2]
    end

    test "en el turno inicial (no puede bajar) solo intercambia" do
      v = view(%{"hand" => hand([0, 1, 2]), "can_lay_down" => false})
      assert %{"type" => "draw", "source" => "hidden"} = Strategy.turn_action(v, none())
    end

    test "deposita en el banco de su figura, normales antes que comodines" do
      bank = %{"id" => 0, "owner" => "bot", "figure" => 2, "cards" => hand([8, 9, 10])}

      v =
        view(%{"hand" => hand([56, 11, 30]), "laid_down" => true, "can_deposit" => true, "banks" => [bank]})

      assert %{"type" => "deposit", "card" => 11, "bank" => 0} = Strategy.turn_action(v, none())

      assert %{"type" => "deposit", "card" => 56, "bank" => 0} =
               Strategy.turn_action(v, MapSet.new([{:deposit, 11}]))
    end

    test "toma del banco de vistas solo si la carta le sirve" do
      base = %{"hand" => hand([0, 20]), "can_take_discard" => true}

      assert %{"source" => "discard"} =
               Strategy.turn_action(view(Map.put(base, "top_discard", card(1))), none())

      assert %{"source" => "discard"} =
               Strategy.turn_action(view(Map.put(base, "top_discard", card(57))), none())

      assert %{"source" => "hidden"} =
               Strategy.turn_action(view(Map.put(base, "top_discard", card(40))), none())
    end

    test "si Go rechaza la jugada, sigue con la siguiente alternativa" do
      v =
        view(%{
          "hand" => hand([0, 1, 2]),
          "can_lay_down" => true,
          "can_take_discard" => true,
          "top_discard" => card(3)
        })

      assert %{"type" => "lay_down"} = Strategy.turn_action(v, none())
      assert %{"source" => "discard"} = Strategy.turn_action(v, MapSet.new([:lay_down]))
      assert %{"source" => "hidden"} = Strategy.turn_action(v, MapSet.new([:lay_down, :take_discard]))
      assert Strategy.turn_action(v, MapSet.new([:lay_down, :take_discard, :draw])) == nil
    end

    test "bota la carta suelta y nunca un comodín si hay alternativa" do
      v = view(%{"phase" => "awaiting_discard", "hand" => hand([0, 1, 56, 40])})

      for _ <- 1..20 do
        assert %{"type" => "discard", "card" => 40} = Strategy.turn_action(v, none())
      end
    end

    test "no desarma una jugada ya armada al botar" do
      # Tiene el trío 0,1,2 y dos sueltas de la figura 3: bota una de las sueltas.
      v = view(%{"phase" => "awaiting_discard", "hand" => hand([0, 1, 2, 16, 17])})

      for _ <- 1..20 do
        assert %{"card" => card} = Strategy.turn_action(v, none())
        assert card in [16, 17]
      end
    end

    test "después de bajar conserva lo que puede depositar" do
      bank = %{"id" => 0, "owner" => "x", "figure" => 1, "cards" => hand([0, 1, 2])}

      v =
        view(%{"phase" => "awaiting_discard", "laid_down" => true, "banks" => [bank], "hand" => hand([3, 40])})

      assert %{"card" => 40} = Strategy.turn_action(v, none())
    end

    test "fuera de su turno o de una ronda en juego no hace nada" do
      refute Strategy.my_turn?(view(%{"current_turn" => "otro"}))
      refute Strategy.my_turn?(view(%{"phase" => "round_over"}))
      assert Strategy.turn_action(view(%{"phase" => "round_over"}), none()) == nil
    end
  end

  describe "robo" do
    @calm %{aggression: 1.0, pace: 1.0}

    test "probabilidad según lo que aporta la carta" do
      base = %{"can_steal" => true, "current_turn" => "otro", "hand" => hand([0, 1, 20])}
      assert Strategy.steal_chance(view(Map.put(base, "top_discard", card(2)))) > 0.3
      assert Strategy.steal_chance(view(Map.put(base, "top_discard", card(58)))) > 0.3
      assert Strategy.steal_chance(view(Map.put(base, "top_discard", card(40)))) == 0.0
    end

    test "no roba después de bajar ni si el castigo puede cerrar la ronda" do
      base = %{"can_steal" => true, "top_discard" => card(58), "hand" => hand([0, 1])}
      assert Strategy.steal_chance(view(Map.put(base, "laid_down", true))) == 0.0
      assert Strategy.steal_chance(view(Map.put(base, "hidden_count", 1))) == 0.0
    end

    test "solo roba si Go lo permite" do
      v = view(%{"can_steal" => false, "top_discard" => card(58), "hand" => hand([0])})
      refute Strategy.steal?(v, @calm, 0.0)
      assert Strategy.steal?(%{v | "can_steal" => true}, @calm, 0.0)
    end

    test "la clave cambia con cada carta botada" do
      a = view(%{"discard_count" => 3, "top_discard" => card(5)})
      b = view(%{"discard_count" => 4, "top_discard" => card(9)})
      refute Strategy.steal_key(a) == Strategy.steal_key(b)
    end
  end
end
