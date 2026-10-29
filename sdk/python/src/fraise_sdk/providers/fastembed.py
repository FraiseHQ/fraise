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
"""A local embedding provider on fastembed.

``fastembed`` is optional. It ships with the ``fastembed`` extra
(``pip install 'fraise-sdk[fastembed]'``). The package does not import this
module, so the core SDK does not depend on it. Import the provider from here.
"""

from __future__ import annotations

from collections.abc import Sequence

from fraise_sdk.providers.base import Embedder

try:
    import fastembed
except ImportError as exc:  # pragma: no cover - exercised only without the extra
    raise ImportError(
        "The fastembed provider requires the 'fastembed' extra. "
        "Install it with:  pip install 'fraise-sdk[fastembed]'"
    ) from exc


class FastEmbedEmbedder(Embedder):
    """Text embeddings computed in process by fastembed, on ONNX Runtime.

    Nothing leaves the machine and nothing needs a key: the model runs locally,
    with no torch. The default model is ``BAAI/bge-small-en-v1.5``, with 384
    dimensions. ``cache_dir`` is where the model's files are kept. Without it,
    fastembed's own default applies, and ``FASTEMBED_CACHE_PATH`` overrides
    that. Requires the ``fastembed`` extra::

        pip install 'fraise-sdk[fastembed]'

    The model loads on the first :meth:`embed`, not at construction. Loading
    reads the weights, and downloads them the first time, which takes seconds.
    So a process that never encodes anything, such as a short-lived tool
    server answering an anchor-only recall, never pays for it.
    """

    def __init__(
        self,
        model: str = "BAAI/bge-small-en-v1.5",
        *,
        cache_dir: str | None = None,
    ) -> None:
        self._model_name = model
        self._cache_dir = cache_dir
        self._model: fastembed.TextEmbedding | None = None

    def embed(self, text: str) -> Sequence[float]:
        """Encode ``text`` into a flat list of floats, loading the model on first use.

        Returns:
            The model's sentence embedding, one float per dimension.
        """
        if self._model is None:
            self._model = fastembed.TextEmbedding(
                self._model_name, cache_dir=self._cache_dir
            )
        # embed takes a batch and yields one numpy array per text. tolist()
        # also converts its numpy scalars to plain floats, which json.dumps
        # can serialize.
        (vector,) = self._model.embed([text])
        return [float(value) for value in vector.tolist()]
