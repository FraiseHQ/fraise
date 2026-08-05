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

"""An OpenAI embeddings provider.

Optional: the ``openai`` client is imported lazily and ships with the ``openai``
extra (``pip install 'fraise-sdk[openai]'``), so the core SDK depends on neither.
"""

from __future__ import annotations

from collections.abc import Sequence

from . import Embedder


class OpenAIEmbedder(Embedder):
    """Text embeddings from OpenAI's embeddings API.

    Wraps ``client.embeddings.create`` for one text at a time. The default model
    is ``text-embedding-3-small``; ``dimensions`` optionally truncates the vector
    (supported by the ``text-embedding-3-*`` models) so it can match a graph's
    fixed embedding size.

    Pass your own configured ``openai.OpenAI`` client, or let one be built from
    the environment (``OPENAI_API_KEY``, optionally overridden by ``api_key``).
    Requires the ``openai`` extra::

        pip install 'fraise-sdk[openai]'
    """

    def __init__(
        self,
        model: str = "text-embedding-3-small",
        *,
        client: object | None = None,
        dimensions: int | None = None,
        api_key: str | None = None,
    ) -> None:
        if client is None:
            try:
                import openai
            except ImportError as exc:  # pragma: no cover - only without the extra
                raise ImportError(
                    "OpenAIEmbedder requires the 'openai' extra. "
                    "Install it with:  pip install 'fraise-sdk[openai]'"
                ) from exc
            client = openai.OpenAI(api_key=api_key)
        self._client = client
        self._model = model
        self._dimensions = dimensions

    def embed(self, text: str) -> Sequence[float]:
        """Encode ``text`` into a flat list of floats."""
        kwargs: dict[str, object] = {"model": self._model, "input": text}
        if self._dimensions is not None:
            kwargs["dimensions"] = self._dimensions
        response = self._client.embeddings.create(**kwargs)
        return list(response.data[0].embedding)
