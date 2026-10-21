from __future__ import annotations

import pytest

from tests.conftest import SERVICE_HEADERS


@pytest.mark.asyncio
async def test_auth_internal_session_is_service_protected_and_idempotent(ctx):
    payload = {"accountId": "account-123", "nickname": "ana"}

    denied = await ctx.client.post("/internal/auth/session", json=payload)
    assert denied.status_code == 401

    first = await ctx.client.post("/internal/auth/session", json=payload, headers=SERVICE_HEADERS)
    second = await ctx.client.post(
        "/internal/auth/session",
        json={**payload, "nickname": "a-different-name"},
        headers=SERVICE_HEADERS,
    )

    assert first.status_code == second.status_code == 201
    assert first.json()["player"] == second.json()["player"]
    assert second.json()["player"]["nickname"] == "ana"
    current = await ctx.client.get(
        "/api/session", headers={"Authorization": f"Bearer {second.json()['token']}"}
    )
    assert current.status_code == 200
    assert current.json()["player"] == first.json()["player"]
