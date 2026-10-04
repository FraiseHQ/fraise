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

"""HuggingFaceEmbedder tests against a mocked inference client — no network."""

from unittest.mock import MagicMock, patch

import pytest
from fraise_sdk.providers.base import Embedder

# The provider imports its client at module scope, so skip the whole file when
# the optional 'huggingface' extra is not installed.
pytest.importorskip("huggingface_hub", reason="requires the 'huggingface' extra")

from fraise_sdk.providers.huggingface import HuggingFaceEmbedder  # noqa: E402


def test_huggingface_embedder_calls_client_and_returns_vector(
    inference_client, huggingface_default_model
):
    """embed sends the text, model and options and returns the vector."""
    client = inference_client()
    embedder = HuggingFaceEmbedder(
        model=huggingface_default_model, client=client, dimensions=3, normalize=True
    )
    assert embedder.embed("hello") == [0.1, 0.2, 0.3]
    client.feature_extraction.assert_called_once_with(
        "hello", model=huggingface_default_model, dimensions=3, normalize=True
    )


def test_huggingface_embedder_is_callable_and_omits_unset_options(
    inference_client, huggingface_default_model
):
    """Calling the embedder embeds, under the default model and with no options.

    Options left unset are not sent, so the endpoint's own defaults apply.
    """
    client = inference_client()
    HuggingFaceEmbedder(client=client)("world")  # __call__ from the Embedder ABC
    client.feature_extraction.assert_called_once_with(
        "world", model=huggingface_default_model
    )


def test_huggingface_embedder_is_an_embedder(inference_client):
    """HuggingFaceEmbedder satisfies the Embedder contract the client resolves."""
    assert isinstance(HuggingFaceEmbedder(client=inference_client()), Embedder)


def test_huggingface_embedder_returns_plain_floats(inference_client):
    """Whatever numbers tolist() yields, ints included, come back as plain floats."""
    vector = HuggingFaceEmbedder(client=inference_client(values=[0, 1])).embed("hello")
    assert vector == [0.0, 1.0]
    assert all(type(value) is float for value in vector)


def test_huggingface_embedder_rejects_token_level_embeddings(inference_client):
    """A model answering with one vector per token is refused: a fact stores one vector."""
    embedder = HuggingFaceEmbedder(
        client=inference_client(values=[[0.1, 0.2], [0.3, 0.4]])
    )
    with pytest.raises(ValueError, match="token-level embeddings"):
        embedder.embed("hello")


def test_huggingface_embedder_accepts_a_plain_list(inference_client):
    """A client returning a bare list (no .tolist) still works."""
    client = inference_client()
    client.feature_extraction.return_value = [0.5, 0.6]
    assert HuggingFaceEmbedder(client=client).embed("hi") == [0.5, 0.6]


def test_huggingface_embedder_builds_its_own_client_from_the_api_key(inference_client):
    """Without an injected client the embedder builds its own InferenceClient.

    The vendor module is patched where the provider imported it, so the test
    pins what the provider builds without a real client or an API key.
    """
    hub = MagicMock()
    hub.InferenceClient.return_value = inference_client()
    with patch("fraise_sdk.providers.huggingface.huggingface_hub", hub):
        embedder = HuggingFaceEmbedder(api_key="hf-test")
    hub.InferenceClient.assert_called_once_with(api_key="hf-test")
    assert embedder.embed("hello") == [0.1, 0.2, 0.3]
