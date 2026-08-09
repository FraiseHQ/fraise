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

"""A Hugging Face feature-extraction provider.

Optional: the ``huggingface_hub`` client is imported lazily and ships with the
``huggingface`` extra (``pip install 'fraise-sdk[huggingface]'``), so the core
SDK depends on neither.
"""

from __future__ import annotations

from collections.abc import Sequence

from fraise_sdk.providers.base import Embedder


class HuggingFaceEmbedder(Embedder):
    """Text embeddings from Hugging Face's feature-extraction inference API.

    Wraps ``client.feature_extraction`` for one text at a time. The default model
    is ``sentence-transformers/all-MiniLM-L6-v2`` (384 dimensions); ``dimensions``
    optionally truncates the vector so it can match a graph's fixed embedding
    size, and ``normalize`` asks for unit-length vectors, which makes cosine
    similarity and dot product agree.

    Pass your own configured ``huggingface_hub.InferenceClient``, or let one be
    built from the environment (``HF_TOKEN``, optionally overridden by
    ``api_key``). Requires the ``huggingface`` extra::

        pip install 'fraise-sdk[huggingface]'

    Only sentence-level models belong here. A model that emits one vector per
    token has no single embedding to return, and :meth:`embed` rejects it rather
    than pick a pooling strategy on your behalf.
    """

    def __init__(
        self,
        model: str = "sentence-transformers/all-MiniLM-L6-v2",
        *,
        client: object | None = None,
        dimensions: int | None = None,
        normalize: bool | None = None,
        api_key: str | None = None,
    ) -> None:
        if client is None:
            try:
                import huggingface_hub
            except ImportError as exc:  # pragma: no cover - only without the extra
                raise ImportError(
                    "HuggingFaceEmbedder requires the 'huggingface' extra. "
                    "Install it with:  pip install 'fraise-sdk[huggingface]'"
                ) from exc
            client = huggingface_hub.InferenceClient(api_key=api_key)
        self._client = client
        self._model = model
        self._dimensions = dimensions
        self._normalize = normalize

    def embed(self, text: str) -> Sequence[float]:
        """Encode ``text`` into a flat list of floats.

        Returns:
            The model's sentence embedding, one float per dimension.

        Raises:
            ValueError: if the model returns a vector per token rather than one
                for the whole text, which no pooling choice here could resolve
                without silently changing what the caller's vectors mean.
        """
        kwargs: dict[str, object] = {"model": self._model}
        if self._dimensions is not None:
            kwargs["dimensions"] = self._dimensions
        if self._normalize is not None:
            kwargs["normalize"] = self._normalize
        vector = self._client.feature_extraction(text, **kwargs)
        # feature_extraction returns a numpy array; tolist() also flattens the
        # numpy scalars to plain floats, which json.dumps can serialize.
        to_list = getattr(vector, "tolist", None)
        if to_list is not None:
            vector = to_list()
        if len(vector) > 0 and isinstance(vector[0], (list, tuple)):
            raise ValueError(
                f"{self._model!r} returned token-level embeddings "
                f"({len(vector)} vectors), not one per text. Use a "
                "sentence-transformers model, or pool them yourself and pass a "
                "plain callable as the embedder."
            )
        return [float(value) for value in vector]
