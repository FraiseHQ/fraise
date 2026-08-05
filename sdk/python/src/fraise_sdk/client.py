# MIT License

# Copyright (c) 2026 René-Jean Corneille

# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:

# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.

# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

"""Synchronous HTTP client for a Fraise server."""

from __future__ import annotations

import warnings
from collections.abc import Sequence

import requests

from . import query as _query
from .errors import FraiseAPIError, FraiseError
from .models import RecallResult
from .providers import Embedder, EmbedderLike, resolve_embedder

DEFAULT_BASE_URL = "http://localhost:9876"
DEFAULT_TIMEOUT_SECONDS = 10.0

# Server versions this SDK is verified against. Keep in sync with COMPATIBILITY.md
# and bump when a release starts relying on newer server behaviour.
SUPPORTED_SERVER = ">=0.1.0,<0.2.0"
_SERVER_MIN = (0, 1, 0)
_SERVER_MAX_EXCLUSIVE = (0, 2, 0)


def _parse_version(text: str) -> tuple[int, int, int] | None:
    """Parse ``major.minor.patch`` into a tuple, ignoring any pre-release suffix.

    Returns ``None`` when the string is not a recognisable version.
    """
    parts = text.strip().lstrip("v").split(".")
    if len(parts) < 3:
        return None
    out: list[int] = []
    for part in parts[:3]:
        digits = ""
        for char in part:
            if not char.isdigit():
                break
            digits += char
        if not digits:
            return None
        out.append(int(digits))
    return (out[0], out[1], out[2])


class FraiseClient:
    """A thin, synchronous client over the Fraise query API.

    All memory operations funnel through the single ``POST /api/v1/q`` endpoint;
    :meth:`remember` and :meth:`recall` are typed conveniences over it, and
    :meth:`query` is the escape hatch for raw query strings.

    The client owns a :class:`requests.Session` for connection reuse. Use it as a
    context manager (``with FraiseClient() as f: ...``) to close that session, or
    call :meth:`close` explicitly.

    Pass ``embedder`` — an object with an ``embed(text)`` method or a plain
    ``callable(text) -> Sequence[float]`` — to have :meth:`remember` and
    :meth:`recall` encode their text into a vector automatically. It stays
    optional: without one, both operate on text/keywords alone, and any call can
    still override with an explicit ``vector`` or opt out with ``embed=False``.
    """

    def __init__(
        self,
        base_url: str = DEFAULT_BASE_URL,
        *,
        timeout: float = DEFAULT_TIMEOUT_SECONDS,
        session: requests.Session | None = None,
        embedder: Embedder | EmbedderLike | None = None,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        # Track whether we own the session so we only close what we created.
        self._owns_session = session is None
        self._session = session or requests.Session()
        self._embed_fn = resolve_embedder(embedder)

    # -- lifecycle ---------------------------------------------------------

    def close(self) -> None:
        if self._owns_session:
            self._session.close()

    def __enter__(self) -> "FraiseClient":
        return self

    def __exit__(self, *_exc) -> None:
        self.close()

    # -- operations --------------------------------------------------------

    def health(self) -> bool:
        """Return True if the server answers its health check with 200/ok."""
        try:
            response = self._session.get(f"{self.base_url}/", timeout=self.timeout)
        except requests.RequestException:
            return False
        return response.status_code == 200

    def server_version(self) -> str | None:
        """Return the server's reported version, or ``None`` if unavailable.

        Reads the ``version`` field from the health endpoint. ``None`` means the
        server is unreachable, answered non-200, or predates version reporting.
        """
        try:
            response = self._session.get(f"{self.base_url}/", timeout=self.timeout)
        except requests.RequestException:
            return None
        if response.status_code != 200:
            return None
        try:
            body = response.json()
        except ValueError:
            return None
        version = body.get("version") if isinstance(body, dict) else None
        return version if isinstance(version, str) and version else None

    def check_compatibility(self, *, strict: bool = False) -> bool:
        """Verify the live server falls within this SDK's supported range.

        Returns ``True`` when the server version is within :data:`SUPPORTED_SERVER`.
        On a mismatch — or when the version can't be determined — the default is to
        emit a :class:`UserWarning` and return ``False``; pass ``strict=True`` to
        raise :class:`FraiseError` instead. This makes no automatic network calls
        of its own beyond the single health request; call it explicitly when you
        want the guarantee.
        """
        version = self.server_version()
        if version is None:
            message = f"could not determine fraise server version at {self.base_url}"
            if strict:
                raise FraiseError(message)
            warnings.warn(message, stacklevel=2)
            return False

        parsed = _parse_version(version)
        if parsed is None or not (_SERVER_MIN <= parsed < _SERVER_MAX_EXCLUSIVE):
            message = (
                f"fraise server {version} is outside this SDK's supported range "
                f"{SUPPORTED_SERVER}; behaviour may be undefined"
            )
            if strict:
                raise FraiseError(message)
            warnings.warn(message, stacklevel=2)
            return False
        return True

    def remember(
        self,
        value: str,
        *,
        graph: int = 0,
        topics: Sequence[str] | None = None,
        entities: Sequence[str] | None = None,
        vector: Sequence[float] | None = None,
        embed: bool | None = None,
        timeout: float | None = None,
    ) -> None:
        """Store ``value`` as a fact in ``graph``.

        ``topics`` and ``entities`` attach the fact to shared hubs so related
        facts become reachable from one another on recall.

        A vector is attached when one is available: an explicit ``vector`` always
        wins; otherwise, if the client has an embedder, ``value`` is encoded
        automatically. ``embed`` overrides that default per call — ``True`` forces
        encoding (and errors if no embedder is set), ``False`` skips it. The first
        vector written to a graph fixes that graph's dimension; later writes must
        match it.

        Returns nothing on success and raises :class:`FraiseAPIError` if the
        server rejects the write.
        """
        resolved = self._resolve_vector(vector, value, embed)
        text = _query.build_remember(
            value,
            graph=graph,
            topics=topics,
            entities=entities,
            with_vector=resolved is not None,
        )
        parameters = {_query.VECTOR_PARAM: resolved} if resolved is not None else None
        self.query(text, parameters=parameters, timeout=timeout)

    def recall(
        self,
        *keywords: str,
        graph: int = 0,
        query: str | None = None,
        topics: Sequence[str] | None = None,
        entities: Sequence[str] | None = None,
        top: int | None = None,
        depth: int | None = None,
        vector: Sequence[float] | None = None,
        embed: bool | None = None,
        timeout: float | None = None,
    ) -> RecallResult:
        """Search ``graph`` for facts and return them ranked by relevance.

        Pass any number of ``keywords`` positionally. A recall needs at least one
        seed — keywords, a vector, or a ``topics``/``entities`` filter — from
        which the walk explores. ``top`` caps the number of results; ``depth``
        bounds how far the walk leaves the seed.

        For semantic search, a vector is attached the same way as in
        :meth:`remember`: an explicit ``vector`` wins; otherwise, if the client
        has an embedder, the ``query`` phrase (or, absent that, the space-joined
        ``keywords``) is encoded. ``embed`` overrides per call.
        """
        embed_text = query if query is not None else " ".join(keywords)
        resolved = self._resolve_vector(vector, embed_text, embed)
        text = _query.build_recall(
            keywords=list(keywords),
            graph=graph,
            topics=topics,
            entities=entities,
            top=top,
            depth=depth,
            with_vector=resolved is not None,
        )
        parameters = {_query.VECTOR_PARAM: resolved} if resolved is not None else None
        body = self.query(text, parameters=parameters, timeout=timeout)
        results = body.get("results") or {}
        return RecallResult.from_json(results)

    # -- embedding ---------------------------------------------------------

    def _resolve_vector(
        self,
        vector: Sequence[float] | None,
        text: str,
        embed: bool | None,
    ) -> list[float] | None:
        """Decide the vector to send: explicit wins, else encode when asked/able.

        ``embed`` is a three-way switch: ``True`` requires an embedder and always
        encodes, ``False`` never encodes, ``None`` encodes only if an embedder is
        configured and ``text`` is non-empty.
        """
        if vector is not None:
            return [float(x) for x in vector]
        if embed is False:
            return None
        if embed is True and self._embed_fn is None:
            raise FraiseError(
                "embed=True but this client has no embedder; construct it with "
                "FraiseClient(..., embedder=...)"
            )
        if self._embed_fn is None or not text.strip():
            return None
        return [float(x) for x in self._embed_fn(text)]

    def query(
        self,
        text: str,
        *,
        parameters: dict[str, list[float]] | None = None,
        timeout: float | None = None,
    ) -> dict:
        """Send a raw query string and return the decoded JSON body.

        This is the low-level escape hatch behind :meth:`remember` and
        :meth:`recall`; reach for it when you need a query the typed helpers do
        not yet cover. Raises :class:`FraiseAPIError` on any non-2xx response.
        """
        payload: dict[str, object] = {"query": text}
        if parameters:
            payload["parameters"] = parameters

        try:
            response = self._session.post(
                f"{self.base_url}/api/v1/q",
                json=payload,
                timeout=self.timeout if timeout is None else timeout,
            )
        except requests.RequestException as exc:
            raise FraiseError(f"could not reach fraise at {self.base_url}: {exc}") from exc

        # Every response the server produces is JSON — decode first so an error
        # body's ``error`` field can be surfaced verbatim.
        try:
            body = response.json()
        except ValueError:
            body = {}

        if not response.ok:
            message = body.get("error") if isinstance(body, dict) else None
            raise FraiseAPIError(response.status_code, message or response.text or "unknown error")

        return body if isinstance(body, dict) else {}
