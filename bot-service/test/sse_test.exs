defmodule BotService.SSETest do
  use ExUnit.Case, async: true
  alias BotService.SSE

  test "eventos completos y resto pendiente" do
    buffer = "event: lobby\ndata: {\"a\":1}\n\n: ping\n\nevent: game_starting\ndata: {\"gam"

    assert {[{"lobby", ~s({"a":1})}], rest} = SSE.parse(buffer)
    assert {[{"game_starting", ~s({"gameId":"g"})}], ""} = SSE.parse(rest <> "eId\":\"g\"}\n\n")
  end

  test "ignora comentarios y une varias líneas de data" do
    assert {[{"message", "uno\ndos"}], ""} = SSE.parse(": ping\n\ndata: uno\ndata: dos\n\n")
  end

  test "acepta CRLF" do
    assert {[{"lobby_closed", "{}"}], ""} = SSE.parse("event: lobby_closed\r\ndata: {}\r\n\r\n")
  end
end
