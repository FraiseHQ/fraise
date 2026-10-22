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
from collections import Counter, defaultdict, deque
from collections.abc import Callable, Sequence

import requests

from fraise_sdk import query as _query
from fraise_sdk.compat import warn
from fraise_sdk.constants import (
    DEFAULT_BASE_URL,
    DEFAULT_TIMEOUT_SECONDS,
    EXPLAIN_PATH,
    MAX_RECENT,
    MAX_VOCABULARY,
    NO_CONTENT,
    QUERY_PATH,
    SDK_FILES,
    SERVER_MAX_EXCLUSIVE,
    SERVER_MIN,
    STATS_PATH,
    SUPPORTED_SERVER,
)
from fraise_sdk.errors import FraiseAPIError, FraiseError, FraiseWarning
from fraise_sdk.models import GraphStats, RecallResult
from fraise_sdk.providers.base import (
    Anchor,
    Context,
    Embedder,
    EmbedderLike,
    Extractor,
    ExtractorLike,
    resolve_embedder,
    resolve_extractor,
)


def _with_extracted(
    given: Sequence[str] | None, extracted: Sequence[str]
) -> Sequence[str] | None:
    """Return ``given`` plus the extracted values it does not already hold.

    The server lowercases anchors, so values that differ only in case are
    repeats. A bare string is returned as is, so the builder can reject it by
    name.
    """
    if not extracted or isinstance(given, str):
        return given
    merged = list(given or [])
    seen = {value.strip().lower() for value in merged}
    for value in extracted:
        key = value.strip().lower()
        if key and key not in seen:
            seen.add(key)
            merged.append(value.strip())
    return merged


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

    Memory operations go through ``POST /api/v1/q``. :meth:`remember` and
    :meth:`recall` are typed conveniences over it. :meth:`query` is the escape
    hatch for raw query strings. :meth:`explain` sends the query :meth:`recall`
    would send to ``POST /api/v1/explain``, which breaks down every hit's
    score. :meth:`stats` reads ``GET /api/v1/stats`` for how much each graph
    holds.

    Requests go through one :class:`requests.Session` for connection reuse. The
    client creates it unless ``session`` passes one in. Use the client as a
    context manager (``with FraiseClient() as f: ...``) or call :meth:`close`
    to close a session it created. A passed-in session is left open.

    Pass ``embedder`` to have :meth:`remember` and :meth:`recall` encode their
    text into a vector automatically. It is an object with an ``embed(text)``
    method, or a plain ``callable(text) -> Sequence[float]``. Without one, both
    work on text alone. Any call can still pass an explicit ``vector`` or opt
    out with ``embed=False``.

    Pass ``extractor`` to have :meth:`remember` file each fact under the topics
    and entities it finds in the text, beside any given ones. It is an object
    with an ``extract(text, context)`` method, or a plain
    ``callable(text, context) -> Sequence[Anchor]``. The text itself is stored
    verbatim. ``extract=False`` opts a call out. Each message is read in its
    graph's :class:`~fraise_sdk.providers.base.Context`: the latest facts
    stored in that graph, and the anchors extraction filed them under. The
    client keeps one context per graph, in this process only. A new client
    starts every graph's context empty.
    """

    def __init__(
        self,
        base_url: str = DEFAULT_BASE_URL,
        *,
        timeout: float = DEFAULT_TIMEOUT_SECONDS,
        session: requests.Session | None = None,
        embedder: Embedder | EmbedderLike | None = None,
        extractor: Extractor | ExtractorLike | None = None,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self._owns_session = session is None
        self._session = session or requests.Session()
        self._embed_fn = resolve_embedder(embedder)
        self._extract_fn = resolve_extractor(extractor)
        # Each graph's context, built from the facts this client stored there:
        # the latest values, and how many facts carry each extracted anchor.
        # All counts are kept, not just the top ones, so an anchor outside the
        # vocabulary's cap can still grow into it.
        self._recent: defaultdict[int, deque[str]] = defaultdict(
            lambda: deque(maxlen=MAX_RECENT)
        )
        self._vocabulary: defaultdict[int, Counter[Anchor]] = defaultdict(Counter)

    def close(self) -> None:
        """Close the session the client created. A passed-in session stays open."""
        if self._owns_session:
            self._session.close()

    def __enter__(self) -> FraiseClient:
        return self

    def __exit__(self, *_exc) -> None:
        self.close()

    def health(self) -> bool:
        """Return whether the server's health endpoint answers 200. Never raises."""
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
        """Check that the server's version is within :data:`SUPPORTED_SERVER`.

        It makes one health request, and only when called.

        Args:
            strict: raise instead of warning when the version is out of range
                or cannot be read.

        Returns:
            ``True`` if the version is in range. Otherwise a :class:`UserWarning`
            is emitted and ``False`` returned.

        Raises:
            FraiseError: in strict mode, if the version is out of range or
                cannot be read.
        """
        version = self.server_version()
        if version is None:
            message = f"could not determine fraise server version at {self.base_url}"
            if strict:
                raise FraiseError(message)
            warnings.warn(message, stacklevel=2)
            return False

        parsed = _parse_version(version)
        if parsed is None or not (SERVER_MIN <= parsed < SERVER_MAX_EXCLUSIVE):
            message = (
                f"fraise server {version} is outside this SDK's supported range "
                f"{SUPPORTED_SERVER}; behaviour may be undefined"
            )
            if strict:
                raise FraiseError(message)
            warnings.warn(message, stacklevel=2)
            return False
        return True

    def stats(self, *, timeout: float | None = None) -> list[GraphStats]:
        """Return how much each graph holds, one row per graph, in id order.

        It reads ``GET /api/v1/stats``, which the server computes from the live
        graphs on every call. Every allocated graph has a row, empty ones
        included. So the list's length is the server's graph count, and a
        graph's row sits at its id.

        Unlike :meth:`health`, it raises on failure instead of returning a
        value. It raises :class:`FraiseError` on a timeout or an unreachable
        server, and :class:`FraiseAPIError` on any non-2xx response. A caller
        checking a restore or sizing a run must not mistake an unreachable
        server for an empty one.

        Args:
            timeout: per-call override of the client's timeout.

        Returns:
            One :class:`~fraise_sdk.models.GraphStats` per graph.
        """
        _, body = self._request(self._session.get, STATS_PATH, timeout=timeout)
        return [GraphStats.from_json(row) for row in body.get("graphs") or []]

    def remember(
        self,
        value: str,
        *,
        graph: int = 0,
        topics: Sequence[str] | None = None,
        entities: Sequence[str] | None = None,
        vector: Sequence[float] | None = None,
        embed: bool | None = None,
        extract: bool | None = None,
        timeout: float | None = None,
    ) -> None:
        """Store ``value`` as a fact in ``graph``.

        ``topics`` and ``entities`` are the anchors the fact is filed under.
        Facts that share an anchor can reach one another on recall.

        If the client has an extractor, the fact is also filed under the
        anchors it finds in ``value``, after the given ``topics`` and
        ``entities``. ``value`` itself is stored verbatim. ``extract``
        overrides that default per call. ``True`` forces extraction, and errors
        if no extractor is set. ``False`` skips it. A failed extraction costs
        the extracted anchors, never the fact. The fact is stored under the
        given anchors, and a :class:`FraiseWarning` names the failure.

        The extractor reads ``value`` in ``graph``'s context. Once the server
        has stored the fact, the fact joins that context, extracted or not. The
        context's vocabulary counts only the anchors the extractor returned.

        A vector is attached when one is available. An explicit ``vector``
        always wins. Otherwise, if the client has an embedder, ``value`` is
        encoded automatically. ``embed`` overrides that default per call.
        ``True`` forces encoding, and errors if no embedder is set. ``False``
        skips it. The first vector written to a graph fixes that graph's
        dimension. Later writes must match it.

        Returns nothing on success and raises :class:`FraiseAPIError` if the
        server rejects the write.
        """
        topics, entities, found = self._resolve_anchors(
            value, graph, topics, entities, extract
        )
        resolved = self._resolve_vector(vector, value, embed)
        text = _query.build_remember(
            value,
            graph=graph,
            topics=topics,
            entities=entities,
            with_vector=resolved is not None,
        )
        parameters = {_query.VECTOR_PARAM: resolved} if resolved is not None else None
        self._post(text, parameters=parameters, timeout=timeout)
        if self._extract_fn is not None:
            self._learn(graph, value, found)

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

        ``query`` is a whole question, sent as one quoted phrase term. So
        natural language travels verbatim, not as bare words that would collide
        with the grammar's reserved words. Pass any number of ``keywords``
        positionally as extra terms. Each is quoted when it needs to be (see
        :mod:`fraise_sdk.query`). A recall needs at least one seed: a query,
        keywords, a vector, or a ``topics``/``entities`` anchor. ``top`` caps
        the number of results.

        ``depth`` picks the retrieval lane, 0 to 2. Lane 0 searches the text
        and vector indices only. Lane 1 lets topics and entities that clearly
        concentrate the matches transmit to the facts filed under them. Lane 2
        admits them at their fair share, for maximum recall. Without ``depth``,
        the server's configured lane applies. The graph is entered only through
        a named topic or entity. So a lane above 0 has no effect on a recall
        without one, and the recall comes back with a warning.

        For semantic search, a vector is attached as in :meth:`remember`. An
        explicit ``vector`` wins. Otherwise, if the client has an embedder, it
        encodes the ``query`` phrase, or the space-joined ``keywords`` when
        there is no query. ``embed`` overrides this per call.

        Any warnings the server attached are listed on the result's
        ``warnings`` and emitted as :class:`FraiseWarning`. A warning means the
        query ran, but something in it cannot help or may not be what was
        meant.
        """
        return self._recall(
            keywords,
            explain=False,
            graph=graph,
            query=query,
            topics=topics,
            entities=entities,
            top=top,
            depth=depth,
            vector=vector,
            embed=embed,
            timeout=timeout,
        )

    def explain(
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
        """Recall as :meth:`recall` does, with each hit's score explained.

        It takes the same arguments and sends the same query to the server's
        explain route, which runs the same pipeline. Each hit comes back with
        ``contributions``: the per-source sightings (text, vector, graph or
        anchor) its score was folded from. The result also carries the query's
        ``background`` rate. Use it to see why a fact ranked where it did. For
        example, it shows whether a multi-hop miss was reached through the
        graph and ranked out, or never reached at all. The breakdown makes the
        response larger, so use :meth:`recall` when the ranking is all you
        need.

        Returns:
            The ranked result, as :meth:`recall` returns it, with
            ``contributions`` on every hit and ``background`` set.
        """
        return self._recall(
            keywords,
            explain=True,
            graph=graph,
            query=query,
            topics=topics,
            entities=entities,
            top=top,
            depth=depth,
            vector=vector,
            embed=embed,
            timeout=timeout,
        )

    def _recall(
        self,
        keywords: Sequence[str],
        *,
        explain: bool,
        graph: int,
        query: str | None,
        topics: Sequence[str] | None,
        entities: Sequence[str] | None,
        top: int | None,
        depth: int | None,
        vector: Sequence[float] | None,
        embed: bool | None,
        timeout: float | None,
    ) -> RecallResult:
        """Build, send and parse a recall, on the query or the explain route.

        :meth:`recall` and :meth:`explain` share this path. So they build the
        same query string and vector, and an explanation always matches the
        ranking :meth:`recall` would return.
        """
        embed_text = query if query is not None else " ".join(keywords)
        resolved = self._resolve_vector(vector, embed_text, embed)
        text = _query.build_recall(
            keywords=list(keywords),
            graph=graph,
            query=query,
            topics=topics,
            entities=entities,
            top=top,
            depth=depth,
            with_vector=resolved is not None,
        )
        parameters = {_query.VECTOR_PARAM: resolved} if resolved is not None else None
        status, body = self._post(
            text,
            parameters=parameters,
            timeout=timeout,
            path=EXPLAIN_PATH if explain else QUERY_PATH,
        )
        results = body.get("results") or {}
        return RecallResult.from_json(
            results,
            warnings=body.get("warnings"),
            empty=status == NO_CONTENT,
            explain=explain,
        )

    def query(
        self,
        text: str,
        *,
        parameters: dict[str, list[float]] | None = None,
        timeout: float | None = None,
    ) -> dict:
        """Send a raw query string and return the decoded JSON body.

        Use it for queries the typed helpers do not cover. The text is sent
        exactly as written, and the server does the checking.

        The body's shape follows the query. A recall answers with ``results``,
        a write with ``{"status": "ok"}``. A recall of a graph that holds
        nothing is answered 204 with no body, which decodes to ``{}``.
        :meth:`recall` reads that status, so prefer it when you need to tell an
        empty graph from a miss.

        Any ``warnings`` the server attached to a successful response are
        emitted as :class:`FraiseWarning`, as they are for the typed helpers.
        Silence them by category with
        ``warnings.filterwarnings("ignore", category=FraiseWarning)``.

        Raises :class:`FraiseError` on a timeout or an unreachable server, and
        :class:`FraiseAPIError` on any non-2xx response.
        """
        _, body = self._post(text, parameters=parameters, timeout=timeout)
        return body

    def _post(
        self,
        text: str,
        *,
        parameters: dict[str, list[float]] | None = None,
        timeout: float | None = None,
        path: str = QUERY_PATH,
    ) -> tuple[int, dict]:
        """Send a query and return the response status beside its decoded body.

        Every query the client sends goes through here, including
        :meth:`query`'s. It is separate from that method because a 204 carries
        its meaning in the status line: the graph searched holds nothing.
        :meth:`query` returns only the body, which is empty for a 204.

        Args:
            text: the raw query string.
            parameters: out-of-band vector bindings the query references.
            timeout: per-call override of the client's timeout.
            path: the route to post to, the query route unless explaining.

        Returns:
            The HTTP status code and the decoded JSON body, ``{}`` when the
            response carried none. A failure raises as :meth:`_request` says.
        """
        payload: dict[str, object] = {"query": text}
        if parameters:
            payload["parameters"] = parameters
        return self._request(self._session.post, path, timeout=timeout, json=payload)

    def _request(
        self,
        send: Callable[..., requests.Response],
        path: str,
        *,
        timeout: float | None = None,
        **kwargs: object,
    ) -> tuple[int, dict]:
        """Send one request and return the response status beside its decoded body.

        Every call that must not mistake a failure for an answer goes through
        here: the queries, via :meth:`_post`, and :meth:`stats`. Only
        :meth:`health` and :meth:`server_version` read the health endpoint
        themselves, because they answer a failure with a value.

        Args:
            send: the session method for the request's verb, such as
                ``self._session.get``.
            path: the route, relative to the base url.
            timeout: per-call override of the client's timeout.
            **kwargs: passed to ``send`` as they are, e.g. the ``json`` body.

        Returns:
            The HTTP status code and the decoded JSON body, ``{}`` when the
            response carried none.

        Raises:
            FraiseError: if the request times out or the server is unreachable.
            FraiseAPIError: if the server answers with a non-2xx status.
        """
        effective_timeout = self.timeout if timeout is None else timeout
        try:
            response = send(
                f"{self.base_url}{path}", timeout=effective_timeout, **kwargs
            )
        except requests.Timeout as exc:
            raise FraiseError(
                f"fraise at {self.base_url} timed out after {effective_timeout}s — "
                f"large graphs may need a higher timeout (pass timeout=... to "
                f"FraiseClient() or to this call)"
            ) from exc
        except requests.RequestException as exc:
            raise FraiseError(
                f"could not reach fraise at {self.base_url}: {exc}"
            ) from exc

        # Decode first, so an error body's ``error`` field can be surfaced
        # verbatim. A body that is not JSON, such as a 204's or a proxy's error
        # page, decodes to {}.
        try:
            body = response.json()
        except ValueError:
            body = {}

        if not response.ok:
            message = body.get("error") if isinstance(body, dict) else None
            raise FraiseAPIError(
                response.status_code, message or response.text or "unknown error"
            )

        body = body if isinstance(body, dict) else {}

        # Surface the server's warnings through Python's own channel. The query
        # ran and the results are valid, but the server flagged something the
        # caller may want to change. Each message gets its own FraiseWarning,
        # so a caller can react to one or silence them all.
        for message in body.get("warnings") or []:
            warn(message, FraiseWarning, skip_file_prefixes=SDK_FILES)

        return response.status_code, body

    def _resolve_vector(
        self,
        vector: Sequence[float] | None,
        text: str,
        embed: bool | None,
    ) -> list[float] | None:
        """Return the vector to send: an explicit one, else ``text`` encoded, or None.

        ``embed`` is a three-way switch: ``True`` requires an embedder, ``False``
        never encodes, ``None`` encodes only if an embedder is configured. Blank
        ``text`` is never encoded.

        Raises:
            FraiseError: if ``embed`` is ``True`` and the client has no embedder.
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

    def _resolve_anchors(
        self,
        value: str,
        graph: int,
        topics: Sequence[str] | None,
        entities: Sequence[str] | None,
        extract: bool | None,
    ) -> tuple[Sequence[str] | None, Sequence[str] | None, list[Anchor]]:
        """Return the anchors to file ``value`` under, and those extracted for it.

        ``extract`` is the same three-way switch as ``embed``: ``True``
        requires an extractor, ``False`` never extracts, ``None`` extracts only
        if one is configured. The extractor reads ``value`` in ``graph``'s
        context. If it fails, only the extracted anchors are lost. A failure is
        a raised error, or an answer that is not a list of anchors. The given
        anchors are returned, and a :class:`FraiseWarning` says why. So the
        fact is never lost to its tagging.

        Returns:
            The topics and the entities to file ``value`` under, then the
            extracted anchors alone. The topics and entities hold the given
            values first, then the extracted ones. The extracted anchors are
            empty when nothing was extracted.

        Raises:
            FraiseError: if ``extract`` is ``True`` and the client has no
                extractor.
        """
        if extract is False:
            return topics, entities, []
        if extract is True and self._extract_fn is None:
            raise FraiseError(
                "extract=True but this client has no extractor; construct it with "
                "FraiseClient(..., extractor=...)"
            )
        if self._extract_fn is None:
            return topics, entities, []
        try:
            # Each anchor's fields are read here, inside the try, so an answer
            # that is not a list of anchors fails as an extraction.
            found = [
                Anchor(value=anchor.value, type=anchor.type)
                for anchor in self._extract_fn(value, self._context(graph))
            ]
        except Exception as exc:
            warn(
                f"anchor extraction failed ({exc!r}); the fact is stored without "
                "extracted anchors",
                FraiseWarning,
                skip_file_prefixes=SDK_FILES,
            )
            return topics, entities, []
        return (
            _with_extracted(topics, [a.value for a in found if a.type == "topic"]),
            _with_extracted(entities, [a.value for a in found if a.type == "entity"]),
            found,
        )

    def _context(self, graph: int) -> Context:
        """Return ``graph``'s context, built from the facts stored there so far."""
        return Context(
            recent=tuple(self._recent[graph]),
            vocabulary=tuple(
                anchor
                for anchor, _ in self._vocabulary[graph].most_common(MAX_VOCABULARY)
            ),
        )

    def _learn(self, graph: int, value: str, found: Sequence[Anchor]) -> None:
        """Add a stored fact, and the anchors extracted for it, to ``graph``'s context.

        An anchor is counted as the server stores it, trimmed and in lower
        case. It counts once per fact, so the most used anchor is the one filed
        on the most facts. A tie keeps the order of first use, so a graph's
        history produces the same prompt on every run.
        """
        self._recent[graph].append(value)
        filed = dict.fromkeys(
            Anchor(value=anchor.value.strip().lower(), type=anchor.type)
            for anchor in found
            if anchor.value.strip()
        )
        self._vocabulary[graph].update(filed.keys())
