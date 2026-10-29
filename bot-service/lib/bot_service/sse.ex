defmodule BotService.SSE do
  @moduledoc """
  Parser incremental de Server-Sent Events. Recibe el búfer acumulado y devuelve
  los eventos completos y el resto que todavía no termina en línea en blanco.
  """

  @spec parse(String.t()) :: {[{String.t(), String.t()}], String.t()}
  def parse(buffer) do
    parts = buffer |> String.replace("\r\n", "\n") |> String.split("\n\n")
    {complete, [rest]} = Enum.split(parts, -1)
    {complete |> Enum.map(&parse_block/1) |> Enum.reject(&is_nil/1), rest}
  end

  # Los comentarios (": ping") y los bloques sin data no generan evento.
  defp parse_block(block) do
    {event, data} =
      block
      |> String.split("\n")
      |> Enum.reduce({"message", []}, fn
        ":" <> _, acc -> acc
        "event:" <> name, {_, data} -> {String.trim(name), data}
        "data:" <> value, {event, data} -> {event, [strip_space(value) | data]}
        _, acc -> acc
      end)

    if data == [], do: nil, else: {event, data |> Enum.reverse() |> Enum.join("\n")}
  end

  defp strip_space(" " <> value), do: value
  defp strip_space(value), do: value
end
