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

"""OpenAI provider tests against a mocked openai client — no vendor calls."""

import sys
from unittest.mock import MagicMock, patch

import pytest
from fraise_sdk import FraiseClient, FraiseError
from fraise_sdk.providers.base import Anchor, Embedder, Extractor
from fraise_sdk.providers.openai import OpenAIEmbedder, OpenAIExtractor

DEFAULT_MODEL = "text-embedding-3-small"


def _client(embedding=(0.1, 0.2, 0.3)) -> MagicMock:
    """A mock ``openai.OpenAI`` answering with one embedding."""
    client = MagicMock()
    client.embeddings.create.return_value = MagicMock(
        data=[MagicMock(embedding=list(embedding))]
    )
    return client


def test_openai_embedder_calls_client_and_returns_vector():
    client = _client()
    embedder = OpenAIEmbedder(model=DEFAULT_MODEL, client=client, dimensions=3)
    assert embedder.embed("hello") == [0.1, 0.2, 0.3]
    client.embeddings.create.assert_called_once_with(
        model=DEFAULT_MODEL, input="hello", dimensions=3
    )


def test_openai_embedder_is_callable_and_omits_dimensions_when_unset():
    client = _client()
    OpenAIEmbedder(client=client)("world")  # __call__ inherited from the Embedder ABC
    client.embeddings.create.assert_called_once_with(model=DEFAULT_MODEL, input="world")


def test_openai_embedder_is_an_embedder():
    assert isinstance(OpenAIEmbedder(client=_client()), Embedder)


def test_openai_embedder_builds_its_own_client_from_the_api_key():
    """Without an injected client the embedder imports openai and builds one.

    The import is lazy and lives inside ``__init__``, so it is patched in
    ``sys.modules`` — that keeps the test running whether or not the optional
    'openai' extra is installed.
    """
    openai = MagicMock()
    openai.OpenAI.return_value = _client()
    with patch.dict(sys.modules, {"openai": openai}):
        embedder = OpenAIEmbedder(api_key="sk-test")
    openai.OpenAI.assert_called_once_with(api_key="sk-test")
    assert embedder.embed("hello") == [0.1, 0.2, 0.3]


def test_openai_extractor_turns_the_answer_into_anchors(chat_client):
    """Each value in the model's answer becomes one Anchor, topics first."""
    client = chat_client(
        '{"topics": ["travel"], "entities": ["Anne", "Lisbon airport"]}'
    )

    anchors = OpenAIExtractor(client=client).extract("Anne lands at Lisbon airport")

    assert anchors == [
        Anchor(value="travel", type="topic"),
        Anchor(value="Anne", type="entity"),
        Anchor(value="Lisbon airport", type="entity"),
    ]


def test_openai_extractor_sends_the_message_verbatim(chat_client):
    """The text reaches the model untouched, under a strict schema and a cap.

    The completion cap is what keeps a reasoning model's hidden tokens from
    truncating the JSON answer mid-string; the strict schema is what makes the
    answer parseable at all. Both are part of the request the benchmark makes.
    """
    client = chat_client('{"topics": [], "entities": []}')
    text = "it's 3:30 — Anne's flight lands"

    OpenAIExtractor(client=client).extract(text)

    request = client.chat.completions.create.call_args.kwargs
    assert request["model"] == "gpt-5-mini"
    assert request["messages"][-1] == {"role": "user", "content": text}
    assert request["max_completion_tokens"] == 16000
    assert request["response_format"]["json_schema"]["strict"] is True


@pytest.mark.parametrize("content", [None, '{"topics": ["tra', '{"topics": []}', "[]"])
def test_openai_extractor_raises_on_an_answer_it_cannot_read(chat_client, content):
    """A refusal, truncated JSON or the wrong shape raises, naming finish_reason.

    The extractor does not pretend it found nothing: the client is what turns
    the failure into a warning and stores the fact anyway, and finish_reason
    is what tells a truncated answer from a refusal.
    """
    client = chat_client(content, finish_reason="length")

    with pytest.raises(FraiseError, match="finish_reason=length"):
        OpenAIExtractor(client=client).extract("the kettle whistles")


def test_openai_extractor_is_an_extractor(chat_client):
    """The provider satisfies the contract the client resolves."""
    assert isinstance(OpenAIExtractor(client=chat_client("{}")), Extractor)


def test_openai_extractor_builds_its_own_client_from_the_api_key(chat_client):
    """Without an injected client the extractor imports openai and builds one.

    The import is lazy and lives inside ``__init__``, so it is patched in
    ``sys.modules`` — that keeps the test running whether or not the optional
    'openai' extra is installed.
    """
    openai = MagicMock()
    openai.OpenAI.return_value = chat_client('{"topics": ["birds"], "entities": []}')
    with patch.dict(sys.modules, {"openai": openai}):
        extractor = OpenAIExtractor(api_key="sk-test")

    openai.OpenAI.assert_called_once_with(api_key="sk-test")
    assert extractor.extract("the heron fishes") == [
        Anchor(value="birds", type="topic")
    ]


# -- integration --------------------------------------------------------------


@pytest.mark.integration
def test_openai_anchors_file_the_fact_on_the_live_server(
    fraise_url, round_trip_graph, chat_client
):
    """What the model answers is what the live server files the fact under.

    The model is scripted — the suite makes no vendor calls — so this pins
    the rest of the path: the answer's JSON, parsed into anchors, carried by
    the client into a remember, and recalled by those anchors, multi-word
    values included.
    """
    answer = '{"topics": ["bird migration"], "entities": ["Ptolemy the swallow"]}'
    fact = "Ptolemy the swallow crossed the Sahara in 40 hours"
    extractor = OpenAIExtractor(client=chat_client(answer))
    with FraiseClient(fraise_url, extractor=extractor) as client:
        client.remember(fact, graph=round_trip_graph)
        by_topic = client.recall(graph=round_trip_graph, topics=["bird migration"])
        by_entity = client.recall(
            graph=round_trip_graph, entities=["ptolemy the swallow"]
        )

    assert fact in [hit.value for hit in by_topic]
    assert fact in [hit.value for hit in by_entity]
