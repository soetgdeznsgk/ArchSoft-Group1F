defmodule BotService.RouterTest do
  # Comparte el Agent del Lobby falso y el supervisor de bots.
  use ExUnit.Case, async: false
  import Plug.Test
  import Plug.Conn

  alias BotService.{FakeLobby, Router}

  setup do
    on_exit(fn ->
      for {_, pid, _, _} <- DynamicSupervisor.which_children(BotService.BotSupervisor) do
        DynamicSupervisor.terminate_child(BotService.BotSupervisor, pid)
      end
    end)
  end

  defp call(method, path, token, body \\ nil) do
    conn =
      conn(method, path, body && Jason.encode!(body))
      |> put_req_header("content-type", "application/json")

    conn = if token, do: put_req_header(conn, "authorization", "Bearer " <> token), else: conn
    conn = Router.call(conn, Router.init([]))
    {conn.status, Jason.decode!(conn.resp_body)}
  end

  describe "POST /api/bots/lobbies/:id/fill" do
    test "el anfitrión llena los cupos libres con bots" do
      start_supervised!(%{id: FakeLobby, start: {FakeLobby, :start, [[{"t_ana", "p_ana", "Ana"}]]}})

      {201, body} = call(:post, "/api/bots/lobbies/l_test/fill", "t_ana")

      assert length(body["bots"]) == 3
      assert Enum.all?(body["bots"], &String.starts_with?(&1["nickname"], "Bot "))
      assert body["lobby"]["playerCount"] == 4
      assert hd(FakeLobby.lobby()["players"])["id"] == "p_ana"
    end

    test "count limita cuántos bots se agregan" do
      start_supervised!(%{id: FakeLobby, start: {FakeLobby, :start, [[{"t_ana", "p_ana", "Ana"}]]}})

      {201, body} = call(:post, "/api/bots/lobbies/l_test/fill", "t_ana", %{count: 1})

      assert length(body["bots"]) == 1
      assert body["lobby"]["playerCount"] == 2
    end

    test "nunca supera los cupos libres" do
      players = [{"t_ana", "p_ana", "Ana"}, {"t_beto", "p_beto", "Beto"}]
      start_supervised!(%{id: FakeLobby, start: {FakeLobby, :start, [players]}})

      {201, body} = call(:post, "/api/bots/lobbies/l_test/fill", "t_ana", %{count: 3})

      assert length(body["bots"]) == 2
      assert body["lobby"]["playerCount"] == 4
    end

    test "solo el anfitrión puede llenar la sala" do
      players = [{"t_ana", "p_ana", "Ana"}, {"t_beto", "p_beto", "Beto"}]
      start_supervised!(%{id: FakeLobby, start: {FakeLobby, :start, [players]}})

      assert {403, %{"code" => "not_lobby_host"}} = call(:post, "/api/bots/lobbies/l_test/fill", "t_beto")
    end

    test "rechaza la sala llena, la partida iniciada y la sesión inválida" do
      full = for n <- 1..4, do: {"t_#{n}", "p_#{n}", "Jugador #{n}"}
      start_supervised!(%{id: FakeLobby, start: {FakeLobby, :start, [full]}})
      assert {409, %{"code" => "lobby_full"}} = call(:post, "/api/bots/lobbies/l_test/fill", "t_1")
      assert {401, %{"code" => "invalid_session"}} = call(:post, "/api/bots/lobbies/l_test/fill", "otro")
      assert {401, %{"code" => "invalid_session"}} = call(:post, "/api/bots/lobbies/l_test/fill", nil)
      stop_supervised!(FakeLobby)

      start_supervised!(%{
        id: FakeLobby,
        start: {FakeLobby, :start, [[{"t_ana", "p_ana", "Ana"}], [status: "IN_GAME"]]}
      })

      assert {409, %{"code" => "lobby_started"}} = call(:post, "/api/bots/lobbies/l_test/fill", "t_ana")
    end

    test "valida count y el ID de la sala" do
      start_supervised!(%{id: FakeLobby, start: {FakeLobby, :start, [[{"t_ana", "p_ana", "Ana"}]]}})

      assert {422, %{"code" => "invalid_request"}} =
               call(:post, "/api/bots/lobbies/l_test/fill", "t_ana", %{count: 9})

      assert {404, %{"code" => "lobby_not_found"}} =
               call(:post, "/api/bots/lobbies/l_otra/fill", "t_ana")

      assert {404, _} = call(:post, "/api/bots/lobbies/..%2Fsession/fill", "t_ana")
    end
  end

  test "GET /healthz" do
    assert {200, %{"status" => "ok"}} = call(:get, "/healthz", nil)
  end
end
