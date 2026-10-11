"""Fetch Source with the local Chrome window instead of Python's HTTP client.

Python and curl are rejected with 403 while a normal browser still loads the
same page. This client drives Chrome on the machine where you run it.
"""

from __future__ import annotations

import atexit
import hashlib
import re
import time
from pathlib import Path

from playwright.sync_api import sync_playwright


class BrowserPageClient:
    def __init__(
        self,
        cache_dir: Path,
        min_interval_s: float = 5.0,
        batch_size: int = 10,
        batch_pause_s: float = 15.0,
    ) -> None:
        self.cache_dir = cache_dir
        self.min_interval_s = min_interval_s
        self.batch_size = batch_size
        self.batch_pause_s = batch_pause_s
        self._last_fetch_at = 0.0
        self._network_fetches = 0
        self.cache_dir.mkdir(parents=True, exist_ok=True)
        profile = cache_dir.parent / "quest-chrome"
        profile.mkdir(parents=True, exist_ok=True)
        self._playwright = sync_playwright().start()
        self._context = self._playwright.chromium.launch_persistent_context(
            str(profile),
            channel="chrome",
            headless=False,
        )
        self._page = self._context.pages[0] if self._context.pages else self._context.new_page()
        atexit.register(self.close)

    def close(self) -> None:
        if getattr(self, "_context", None) is None:
            return
        context = self._context
        playwright = self._playwright
        self._context = None
        self._playwright = None
        context.close()
        playwright.stop()

    def get_html(self, url: str, *, force: bool = False) -> str:
        path = self._cache_path(url)
        if not force and path.is_file():
            return path.read_text(encoding="utf-8", errors="replace")

        self._throttle()
        html = self._goto(url)
        if html is None:
            bare = _bare_page_url(url)
            if bare == url:
                raise RuntimeError(f"too many redirects loading {url}")
            print(f"  redirect loop on slugged URL, retrying {bare}", flush=True)
            html = self._goto(bare)
            if html is None:
                raise RuntimeError(f"too many redirects loading {bare}")
        if _looks_blocked(html):
            raise RuntimeError(f"Chrome was blocked loading {url}")
        path.write_text(html, encoding="utf-8")
        self._network_fetches += 1
        if self.batch_size > 0 and self.batch_pause_s > 0 and self._network_fetches % self.batch_size == 0:
            time.sleep(self.batch_pause_s)
        return html

    def _goto(self, url: str) -> str | None:
        try:
            self._page.goto(url, wait_until="domcontentloaded", timeout=120_000)
        except Exception as exc:
            if "ERR_TOO_MANY_REDIRECTS" in str(exc):
                return None
            raise
        return self._page.content()

    def _cache_path(self, url: str) -> Path:
        digest = hashlib.sha256(url.encode("utf-8")).hexdigest()[:32]
        return self.cache_dir / f"{digest}.html"

    def _throttle(self) -> None:
        elapsed = time.monotonic() - self._last_fetch_at
        if elapsed < self.min_interval_s:
            time.sleep(self.min_interval_s - elapsed)
        self._last_fetch_at = time.monotonic()


def _bare_page_url(url: str) -> str:
    match = re.match(r"(https://www\.wowhead\.com/forever/(?:quest|object|item)=\d+)", url)
    if match is None:
        return url
    return match.group(1)


def _looks_blocked(html: str) -> bool:
    sample = html[:8000].lower()
    if len(html) < 500:
        return True
    if "access denied" in sample or "just a moment" in sample:
        return True
    if "wowhead.com" not in sample and "zamimg" not in sample:
        return True
    return False
