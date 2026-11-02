"""El cálculo del puntaje no necesita base de datos."""

from __future__ import annotations

import pytest

from app.elo import Entry, expected_score, rating_changes


def test_con_puntajes_iguales_la_expectativa_es_cincuenta_por_ciento():
    assert expected_score(1000, 1000) == pytest.approx(0.5)


def test_una_ventaja_de_400_puntos_da_diez_a_uno():
    assert expected_score(1400, 1000) == pytest.approx(10 / 11)


def test_duelo_entre_iguales_mueve_la_mitad_de_k():
    changes = rating_changes([Entry("a", 1000, 1), Entry("b", 1000, 2)], 32)
    assert changes == {"a": pytest.approx(16), "b": pytest.approx(-16)}


def test_vencer_a_un_rival_mas_fuerte_da_mas_puntos():
    upset = rating_changes([Entry("a", 1000, 1), Entry("b", 1400, 2)], 32)["a"]
    expected = rating_changes([Entry("a", 1400, 1), Entry("b", 1000, 2)], 32)["a"]
    assert upset > expected > 0


def test_el_empate_entre_iguales_no_cambia_nada():
    changes = rating_changes([Entry("a", 1000, 1), Entry("b", 1000, 1)], 32)
    assert changes == {"a": pytest.approx(0), "b": pytest.approx(0)}


@pytest.mark.parametrize(
    "entries",
    [
        [Entry("a", 1000, 1), Entry("b", 1000, 2), Entry("c", 1000, 3), Entry("d", 1000, 4)],
        [Entry("a", 1312.5, 2), Entry("b", 987, 1), Entry("c", 1100, 2), Entry("d", 640, 4)],
        [Entry("a", 1500, 3), Entry("b", 900, 1), Entry("c", 1200, 2)],
    ],
)
def test_ningun_punto_se_crea_ni_se_pierde(entries):
    assert sum(rating_changes(entries, 32).values()) == pytest.approx(0)


def test_el_cambio_maximo_no_depende_del_numero_de_jugadores():
    table = [Entry(name, 1000, rank) for rank, name in enumerate("abcd", start=1)]
    changes = rating_changes(table, 32)
    assert changes["a"] == pytest.approx(16)
    assert changes["d"] == pytest.approx(-16)
    assert changes["b"] == pytest.approx(16 / 3)
