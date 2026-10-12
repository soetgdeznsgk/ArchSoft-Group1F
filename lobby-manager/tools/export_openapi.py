"""Regenera docs/openapi.json a partir del código. No necesita base de datos.

    python tools/export_openapi.py
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT))

from app.config import Settings  # noqa: E402
from app.main import create_app  # noqa: E402

app = create_app(Settings(database_url="", jwt_secret="no-se-usa", service_secret="no-se-usa"))
target = ROOT / "docs" / "openapi.json"
target.write_text(json.dumps(app.openapi(), indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
print(f"Escrito {target.relative_to(ROOT)} ({len(app.openapi()['paths'])} rutas)")
