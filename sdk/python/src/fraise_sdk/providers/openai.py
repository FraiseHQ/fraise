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

"""OpenAI providers: an embedder and an anchor extractor.

Optional: the ``openai`` client is imported lazily and ships with the ``openai``
extra (``pip install 'fraise-sdk[openai]'``), so the core SDK does not depend on
it.
"""

from __future__ import annotations

import json
from collections.abc import Sequence

from fraise_sdk.errors import FraiseError
from fraise_sdk.providers.base import Anchor, Embedder, Extractor

# The instructions and output shape the published benchmark ingests with, word
# for word, so an SDK user's write path is the one the numbers measure.
_EXTRACTION_SYSTEM_PROMPT = """\
You are tagging one conversation message for a memory database. Return
the topics the message is about — short tags a later search could filter
on (e.g. "travel", "health") — and the entities it names: the people,
places, organizations, and things. Return empty lists when the message
is pure filler."""

_ANCHORS_SCHEMA = {
    "type": "object",
    "properties": {
        "topics": {"type": "array", "items": {"type": "string"}},
        "entities": {"type": "array", "items": {"type": "string"}},
    },
    "required": ["topics", "entities"],
    "additionalProperties": False,
}

# A reasoning model's hidden reasoning tokens share the completion budget and
# vary from call to call, and a budget they exhaust truncates the JSON
# mid-string. 16k is generous headroom for one message's anchors.
_MAX_COMPLETION_TOKENS = 16000


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


class OpenAIExtractor(Extractor):
    """The topic and entity anchors of one message, from an OpenAI chat model.

    One chat completion per message, constrained to a strict JSON schema of
    ``topics`` and ``entities``, each value becoming one :class:`Anchor`. The
    default model is ``gpt-5-mini``, with the prompt and output shape the
    published benchmark ingests with. The message is only read, never
    rewritten, so what is remembered is what was said.

    Pass your own configured ``openai.OpenAI`` client, or let one be built from
    the environment (``OPENAI_API_KEY``, optionally overridden by ``api_key``).
    Requires the ``openai`` extra::

        pip install 'fraise-sdk[openai]'
    """

    def __init__(
        self,
        model: str = "gpt-5-mini",
        *,
        client: object | None = None,
        api_key: str | None = None,
    ) -> None:
        if client is None:
            try:
                import openai
            except ImportError as exc:  # pragma: no cover - only without the extra
                raise ImportError(
                    "OpenAIExtractor requires the 'openai' extra. "
                    "Install it with:  pip install 'fraise-sdk[openai]'"
                ) from exc
            client = openai.OpenAI(api_key=api_key)
        self._client = client
        self._model = model

    def extract(self, text: str) -> list[Anchor]:
        """Return the topics and entities ``text`` is about, as anchors.

        Raises:
            FraiseError: if the model's answer is empty or not the anchors
                schema — a refusal, or JSON truncated by the completion budget.
        """
        response = self._client.chat.completions.create(
            model=self._model,
            messages=[
                {"role": "system", "content": _EXTRACTION_SYSTEM_PROMPT},
                {"role": "user", "content": text},
            ],
            max_completion_tokens=_MAX_COMPLETION_TOKENS,
            response_format={
                "type": "json_schema",
                "json_schema": {
                    "name": "anchors",
                    "strict": True,
                    "schema": _ANCHORS_SCHEMA,
                },
            },
        )
        choice = response.choices[0]
        try:
            answer = json.loads(choice.message.content)
            return [Anchor(value=v, type="topic") for v in answer["topics"]] + [
                Anchor(value=v, type="entity") for v in answer["entities"]
            ]
        except (TypeError, ValueError, KeyError) as exc:
            raise FraiseError(
                f"anchor extraction answer unparseable (finish_reason="
                f"{choice.finish_reason})"
            ) from exc
