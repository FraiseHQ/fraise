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

The ``openai`` client is optional. It ships with the ``openai`` extra
(``pip install 'fraise-sdk[openai]'``). The package does not import this
module, so the core SDK does not depend on it. Import the providers from here.
"""

from __future__ import annotations

import json
from collections.abc import Sequence

from fraise_sdk.errors import FraiseError
from fraise_sdk.providers.base import Anchor, Context, Embedder, Extractor

try:
    import openai
except ImportError as exc:  # pragma: no cover - exercised only without the extra
    raise ImportError(
        "The OpenAI providers require the 'openai' extra. "
        "Install it with:  pip install 'fraise-sdk[openai]'"
    ) from exc

# The published benchmark ingests with these instructions and this output
# shape, word for word. So an SDK user's write path is the one the numbers
# measure. The prompt describes the fields extract sends. A field renamed on
# one side alone reaches the model unexplained.
_EXTRACTION_SYSTEM_PROMPT = """\
You tag one message from a conversation so that a memory database can link
it to other messages about the same things.

You are given a JSON object with:
- recent: the few messages just before it, for context only. Do not tag them.
- vocabulary: the topics and entities already used in this conversation.
- message: the message to tag.

Return:
- topics: 0 to 3 subjects someone might later ask about: an activity,
  project, plan, event or area of life (for example "adoption", "painting",
  "marathon training"). Lowercase, singular, no possessives. Not moods or
  small talk ("feelings", "greeting", "chat").
- entities: the specific people, pets, places, organizations, events and
  named works the message refers to, including those it refers to by pronoun
  or description ("she", "the agency") when recent makes clear who or what
  they are. People by the name the conversation uses. Not generic objects
  ("a car", "dinner") unless they are named or recur.

Reuse an entry from vocabulary, spelled exactly as it appears there, whenever
it names the same thing. Add a new entry only when nothing in vocabulary fits.
Return empty lists when the message is pure filler."""

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
# vary from call to call. If they exhaust the budget, the JSON is cut off
# mid-string. 16k is generous headroom for one message's anchors.
_MAX_COMPLETION_TOKENS = 16000


class OpenAIEmbedder(Embedder):
    """Text embeddings from OpenAI's embeddings API.

    It wraps ``client.embeddings.create`` for one text at a time. The default
    model is ``text-embedding-3-small``. ``dimensions`` optionally truncates
    the vector to match a graph's fixed embedding size. The
    ``text-embedding-3-*`` models support it.

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

    It makes one chat completion per message, held to a strict JSON schema of
    ``topics`` and ``entities``. Each value becomes one :class:`Anchor`. The
    model reads the message with its graph's :class:`Context`. So a reference
    resolves to what a nearby message named, and a recurring subject is filed
    under the anchor the graph already has. The default model is
    ``gpt-5-mini``, with the prompt and output shape the published benchmark
    ingests with. The message is only read, never rewritten, so what is
    remembered is what was said.

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
            client = openai.OpenAI(api_key=api_key)
        self._client = client
        self._model = model

    def extract(self, text: str, context: Context) -> list[Anchor]:
        """Return the topics and entities ``text`` is about, as anchors.

        The message and its context reach the model as one JSON object. JSON
        carries the text losslessly, quotes and line breaks included. It also
        splits the vocabulary into the topics and the entities an answer can
        reuse.

        Args:
            text: the message to tag.
            context: what the message's graph already holds.

        Raises:
            FraiseError: if the model's answer is empty or not the anchors
                schema, such as a refusal or JSON cut off by the completion
                budget.
        """
        payload = {
            "recent": list(context.recent),
            "vocabulary": {
                "topics": [a.value for a in context.vocabulary if a.type == "topic"],
                "entities": [a.value for a in context.vocabulary if a.type == "entity"],
            },
            "message": text,
        }
        response = self._client.chat.completions.create(
            model=self._model,
            messages=[
                {"role": "system", "content": _EXTRACTION_SYSTEM_PROMPT},
                {"role": "user", "content": json.dumps(payload, ensure_ascii=False)},
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
