defmodule BotService.Strategy do
  @moduledoc """
  Decisiones del bot a partir de la vista que envía Game Logic (`PlayerView` en Go).

  Es una función pura de la vista: no guarda estado ni habla con la red, así que
  cada decisión se puede probar con una vista armada a mano. Las reglas las sigue
  validando Go; si una acción es rechazada, el bot la marca como bloqueada para
  el resto del turno y esta función propone la siguiente alternativa.

  Orden en el turno propio (reglamento, RF-15 a RF-30):

    1. Bajar las jugadas de la ronda, si el momento lo permite y las tiene.
    2. Depositar en los bancos todo lo que encaje (normales antes que comodines).
    3. Tomar del banco de vistas si la carta le sirve; si no, de ocultas.
    4. Botar la carta que menos aporta.

  Fuera de su turno solo decide si roba la última carta botada.
  """
  alias BotService.Melds

  @type personality :: %{aggression: float(), pace: float()}

  @doc """
  Rasgos al azar de cada bot: qué tanto roba (`aggression`) y qué tan rápido
  juega (`pace`, multiplica la pausa). Así no juegan todos igual.
  """
  @spec personality() :: personality()
  def personality do
    %{aggression: 0.6 + :rand.uniform() * 0.8, pace: 0.8 + :rand.uniform() * 0.45}
  end

  def my_turn?(view) do
    view["current_turn"] == view["you"] and view["phase"] in ["awaiting_draw", "awaiting_discard"]
  end

  @doc "Siguiente acción del turno propio, o `nil` si no queda ninguna disponible."
  @spec turn_action(map(), MapSet.t()) :: map() | nil
  def turn_action(view, blocked) do
    case view["phase"] do
      "awaiting_draw" -> lay_down(view, blocked) || deposit(view, blocked) || draw(view, blocked)
      "awaiting_discard" -> discard(view, blocked)
      _other -> nil
    end
  end

  @doc "Lo que se bloquea para el resto del turno cuando Go rechaza la acción."
  def blocked_by(%{"type" => "lay_down"}), do: :lay_down
  def blocked_by(%{"type" => "deposit", "card" => card}), do: {:deposit, card}
  def blocked_by(%{"type" => "draw", "source" => "discard"}), do: :take_discard
  def blocked_by(%{"type" => "draw"}), do: :draw
  def blocked_by(%{"type" => "discard", "card" => card}), do: {:discard, card}
  def blocked_by(%{"type" => "steal"}), do: :steal

  @doc """
  Identifica la carta que está arriba del banco de vistas, para decidir una sola
  vez por carta si se intenta robarla.
  """
  def steal_key(view), do: {view["round"], view["discard_count"], get_in(view, ["top_discard", "id"])}

  @doc "Decide si roba, dado un número aleatorio `roll` en [0, 1)."
  @spec steal?(map(), personality(), float()) :: boolean()
  def steal?(view, personality, roll) do
    view["can_steal"] == true and roll < steal_chance(view) * personality.aggression
  end

  @doc """
  Probabilidad base de robar. Robar trae además una carta oculta de castigo, así
  que solo vale la pena si la carta acerca a bajar. Después de bajar no se roba:
  la mano solo debe achicarse.
  """
  def steal_chance(view) do
    card = view["top_discard"]

    cond do
      card == nil or view["laid_down"] -> 0.0
      # Con menos de 2 ocultas el castigo podría cerrar la ronda sin ganador.
      (view["hidden_count"] || 0) < 2 -> 0.0
      card["joker"] -> 0.55
      true -> figure_count(view["hand"], card["figure"]) |> chance_for_count()
    end
  end

  defp chance_for_count(0), do: 0.0
  defp chance_for_count(1), do: 0.12
  defp chance_for_count(_pair_or_more), do: 0.45

  # --- turno propio ---------------------------------------------------------

  defp lay_down(view, blocked) do
    with true <- view["can_lay_down"] == true and not MapSet.member?(blocked, :lay_down),
         %{"melds" => sizes} <- view["spec"],
         melds when is_list(melds) <- Melds.find(view["hand"], sizes) do
      %{"type" => "lay_down", "melds" => melds}
    else
      _ -> nil
    end
  end

  defp deposit(view, blocked) do
    if view["can_deposit"] == true do
      view["hand"]
      |> Enum.sort_by(& &1["joker"])
      |> Enum.reject(&MapSet.member?(blocked, {:deposit, &1["id"]}))
      |> Enum.find_value(fn card ->
        if bank = bank_for(card, view["banks"]) do
          %{"type" => "deposit", "card" => card["id"], "bank" => bank}
        end
      end)
    end
  end

  defp draw(view, blocked) do
    cond do
      view["can_take_discard"] == true and not MapSet.member?(blocked, :take_discard) and
          wanted?(view["top_discard"], view) ->
        %{"type" => "draw", "source" => "discard"}

      MapSet.member?(blocked, :draw) ->
        nil

      true ->
        %{"type" => "draw", "source" => "hidden"}
    end
  end

  defp wanted?(nil, _view), do: false
  defp wanted?(%{"joker" => true}, _view), do: true

  defp wanted?(card, view) do
    if view["laid_down"],
      do: bank_for(card, view["banks"]) != nil,
      else: figure_count(view["hand"], card["figure"]) >= 1
  end

  defp discard(view, blocked) do
    reserved = reserved_cards(view)

    view["hand"]
    |> Enum.reject(&MapSet.member?(blocked, {:discard, &1["id"]}))
    |> Enum.shuffle()
    |> Enum.min_by(&keep_score(&1, view, reserved), fn -> nil end)
    |> case do
      nil -> nil
      card -> %{"type" => "discard", "card" => card["id"]}
    end
  end

  # Cuanto más alto, más conviene conservar la carta.
  defp keep_score(card, view, reserved) do
    cond do
      card["joker"] -> 1_000
      MapSet.member?(reserved, card["id"]) -> 500
      view["laid_down"] and bank_for(card, view["banks"]) != nil -> 400
      true -> figure_count(view["hand"], card["figure"])
    end
  end

  # Si ya tiene las jugadas armadas (p. ej. en el turno inicial, que no permite
  # bajar), no las desarma al botar.
  defp reserved_cards(%{"laid_down" => false, "spec" => %{"melds" => sizes}, "hand" => hand}) do
    case Melds.find(hand, sizes) do
      nil -> MapSet.new()
      melds -> melds |> List.flatten() |> MapSet.new()
    end
  end

  defp reserved_cards(_view), do: MapSet.new()

  # --- utilidades -----------------------------------------------------------

  defp bank_for(card, banks) do
    Enum.find_value(banks || [], fn bank ->
      if card["joker"] or bank["figure"] == card["figure"], do: bank["id"]
    end)
  end

  defp figure_count(hand, figure) do
    Enum.count(hand, &(not &1["joker"] and &1["figure"] == figure))
  end
end
