"""Parse Source quest list (Listview) pages."""

from __future__ import annotations

import json
import re
from typing import Any

_LISTVIEW_RE = re.compile(
    r"new Listview\(\{template:\s*'quest'.*?data:(\[.*?\])\}\);",
    re.DOTALL,
)


def parse_quest_list(html: str) -> list[dict[str, Any]]:
    match = _LISTVIEW_RE.search(html)
    if not match:
        return []
    payload = match.group(1)
    try:
        data = json.loads(payload)
    except json.JSONDecodeError:
        return []
    if not isinstance(data, list):
        return []
    return [row for row in data if isinstance(row, dict) and "id" in row]
