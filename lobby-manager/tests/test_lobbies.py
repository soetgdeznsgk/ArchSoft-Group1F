"""RF-02 a RF-07: crear, listar, entrar y salir de salas."""

from __future__ import annotations

from app.service import CODE_ALPHABET


async def test_crear_sala_deja_al_jugador_en_la_sala_de_espera(ctx):
    ana = await ctx.login("Ana")
    r = await ctx.client.post("/api/lobbies", headers=ana.headers)
    assert r.status_code == 201
    lobby = r.json()
    assert lobby["status"] == "WAITING"
    assert len(lobby["code"]) == 6 and set(lobby["code"]) <= set(CODE_ALPHABET)
    assert lobby["players"] == [{"id": ana.id, "nickname": "Ana", "seat": 0}]
    assert (lobby["playerCount"], lobby["maxPlayers"]) == (1, 4)
    assert lobby["game"] is None

    me = await ctx.client.get("/api/session", headers=ana.headers)
    assert me.json()["lobby"]["id"] == lobby["id"]


async def test_lista_las_salas_con_codigo_y_cantidad_de_jugadores(ctx):
    visitor = await ctx.login("Visitante")
    assert (await ctx.client.get("/api/lobbies", headers=visitor.headers)).json() == []

    first, _ = await ctx.lobby_with(1)
    second, _ = await ctx.lobby_with(3)
    listed = (await ctx.client.get("/api/lobbies", headers=visitor.headers)).json()
    assert [(l["id"], l["code"], l["playerCount"], l["maxPlayers"]) for l in listed] == [
        (first["id"], first["code"], 1, 4),
        (second["id"], second["code"], 3, 4),
    ]


async def test_maximo_10_salas_simultaneas(ctx):
    owners = []
    for i in range(10):
        owner = await ctx.login(f"Host{i}")
        r = await ctx.client.post("/api/lobbies", headers=owner.headers)
        assert r.status_code == 201
        owners.append((owner, r.json()["id"]))

    extra = await ctx.login("Once")
    r = await ctx.client.post("/api/lobbies", headers=extra.headers)
    assert r.status_code == 409
    assert r.json()["code"] == "lobby_limit_reached"

    # Al vaciarse una sala se elimina y libera su cupo.
    owner, lobby_id = owners[0]
    assert (await ctx.client.post(f"/api/lobbies/{lobby_id}/leave", headers=owner.headers)).status_code == 204
    assert (await ctx.client.post("/api/lobbies", headers=extra.headers)).status_code == 201


async def test_entrar_desde_la_lista_y_por_codigo(ctx):
    lobby, _ = await ctx.lobby_with(1)
    beto, caro = await ctx.login("Beto"), await ctx.login("Caro")

    r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/join", headers=beto.headers)
    assert r.status_code == 200
    assert [p["nickname"] for p in r.json()["players"]] == ["Jugador1", "Beto"]

    r = await ctx.client.post("/api/lobbies/join", json={"code": f" {lobby['code'].lower()} "}, headers=caro.headers)
    assert r.status_code == 200
    assert [(p["nickname"], p["seat"]) for p in r.json()["players"]] == [("Jugador1", 0), ("Beto", 1), ("Caro", 2)]


async def test_codigo_o_sala_inexistente_responde_404(ctx):
    ana = await ctx.login("Ana")
    r = await ctx.client.post("/api/lobbies/join", json={"code": "ZZZZZZ"}, headers=ana.headers)
    assert r.status_code == 404
    assert r.json() == {"code": "lobby_not_found", "message": "No existe una sala con ese código."}
    assert (await ctx.client.post("/api/lobbies/l_nada/join", headers=ana.headers)).status_code == 404
    assert (await ctx.client.get("/api/lobbies/l_nada", headers=ana.headers)).status_code == 404


async def test_sala_llena_rechaza_al_quinto_jugador(ctx):
    lobby, _ = await ctx.lobby_with(4)
    quinto = await ctx.login("Quinto")
    r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/join", headers=quinto.headers)
    assert r.status_code == 409
    assert r.json()["code"] == "lobby_full"


async def test_no_se_puede_entrar_a_una_sala_cuya_partida_inicio(ctx):
    lobby, _ = await ctx.started_lobby()
    tarde = await ctx.login("Tarde")
    r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/join", headers=tarde.headers)
    assert r.status_code == 409
    assert r.json()["code"] == "lobby_started"
    r = await ctx.client.post("/api/lobbies/join", json={"code": lobby["code"]}, headers=tarde.headers)
    assert r.json()["code"] == "lobby_started"


async def test_un_jugador_solo_puede_estar_en_una_sala(ctx):
    first, (ana,) = await ctx.lobby_with(1)
    second, _ = await ctx.lobby_with(1)

    r = await ctx.client.post(f"/api/lobbies/{second['id']}/join", headers=ana.headers)
    assert r.status_code == 409
    assert r.json()["code"] == "already_in_lobby"
    assert (await ctx.client.post("/api/lobbies", headers=ana.headers)).json()["code"] == "already_in_lobby"

    # Repetir el ingreso a la propia sala no es un error ni duplica al jugador.
    r = await ctx.client.post(f"/api/lobbies/{first['id']}/join", headers=ana.headers)
    assert r.status_code == 200
    assert r.json()["playerCount"] == 1


async def test_salir_libera_el_cupo_y_la_sala_vacia_se_elimina(ctx):
    lobby, (ana, beto) = await ctx.lobby_with(2)
    url = f"/api/lobbies/{lobby['id']}"

    assert (await ctx.client.post(f"{url}/leave", headers=ana.headers)).status_code == 204
    after = (await ctx.client.get(url, headers=beto.headers)).json()
    assert [p["id"] for p in after["players"]] == [beto.id]
    assert (await ctx.client.get("/api/session", headers=ana.headers)).json()["lobby"] is None

    # Quien salió ya no pertenece a la sala.
    r = await ctx.client.post(f"{url}/leave", headers=ana.headers)
    assert (r.status_code, r.json()["code"]) == (403, "not_in_lobby")

    assert (await ctx.client.post(f"{url}/leave", headers=beto.headers)).status_code == 204
    assert (await ctx.client.get(url, headers=beto.headers)).status_code == 404
    assert (await ctx.client.get("/api/lobbies", headers=beto.headers)).json() == []
    assert await ctx.pool.fetchval("SELECT count(*) FROM lobbies") == 0


async def test_quien_entra_despues_de_una_salida_recibe_un_asiento_nuevo(ctx):
    lobby, (ana, beto, caro) = await ctx.lobby_with(3)
    await ctx.client.post(f"/api/lobbies/{lobby['id']}/leave", headers=beto.headers)
    r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/join", headers=beto.headers)
    assert [(p["id"], p["seat"]) for p in r.json()["players"]] == [(ana.id, 0), (caro.id, 2), (beto.id, 3)]


async def test_no_se_puede_salir_con_la_partida_iniciada(ctx):
    lobby, users = await ctx.started_lobby()
    r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/leave", headers=users[1].headers)
    assert (r.status_code, r.json()["code"]) == (409, "lobby_started")
