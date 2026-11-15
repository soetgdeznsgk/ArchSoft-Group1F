"""Emite una sesión de invitado como lo haría Auth, para probar el Lobby sin levantar Auth.

    python tools/dev_session.py Ana

Firma con la clave privada de sesiones (por defecto secrets/jwt/session-private.pem, la que
genera scripts/gen-jwt-keys.sh). Solo para desarrollo: en el despliegue solo Auth la tiene.
Variables: SESSION_PRIVATE_KEY_FILE.
"""

from __future__ import annotations

import os
import secrets
import sys
import time
from pathlib import Path

import jwt

ROOT = Path(__file__).resolve().parent.parent.parent
KEY_FILE = Path(os.environ.get("SESSION_PRIVATE_KEY_FILE", ROOT / "secrets" / "jwt" / "session-private.pem"))

if len(sys.argv) != 2:
    sys.exit("Uso: python tools/dev_session.py <nombre>")

now = int(time.time())
claims = {
    "sub": f"p_{secrets.token_hex(12)}",
    "scope": "session",
    "nickname": sys.argv[1],
    "iat": now,
    "exp": now + 12 * 3600,
}
print(jwt.encode(claims, KEY_FILE.read_text(encoding="ascii"), algorithm="RS256"))
