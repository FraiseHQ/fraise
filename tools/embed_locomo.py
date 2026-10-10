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

"""Embed every text the retrieval gate stores or asks, once.

BenchmarkRetrievalQuality (pkg/server/locomo_bench_test.go) reads the
result through FRAISE_LOCOMO_VECTORS and stops on any text it finds no vector
for, so the values built here must be the ones it builds: the amb adapter's
raw path, one fact per turn, valued ``[session N @ <date>] <speaker>: <text>``
with any shared image's caption appended and the ASCII apostrophe swapped for
a typographic one, and each question as written. A vector is the model's
attention-masked mean over tokens, L2-normalized, the pooling the e2e suite
uses. Base and head always read the same file, so they are compared on
identical vectors.

Run with the tests member's ``embeddings`` extra (``make perf-vectors``):

    python tools/embed_locomo.py <locomo.json> <vectors.json>
"""

import argparse
import json
from collections.abc import Iterator
from pathlib import Path

import torch
import transformers

MODEL = "sentence-transformers/all-MiniLM-L6-v2"
BATCH = 64


def texts(conversations: list[dict]) -> Iterator[str]:
    """Every value the benchmark stores and every question it asks, in order.

    Args:
        conversations: LoCoMo conversations, in the dataset's own format.

    Yields:
        The texts the benchmark looks a vector up for.
    """
    for conv in conversations:
        sessions = conv["conversation"]
        n = 1
        while f"session_{n}" in sessions:
            header = f"[session {n} @ {sessions[f'session_{n}_date_time']}]"
            for turn in sessions[f"session_{n}"]:
                text = turn["text"]
                if turn.get("blip_caption"):
                    text += f" [shared image: {turn['blip_caption']}]"
                yield f"{header} {turn['speaker']}: {text}".replace("'", "’")
            n += 1
        for qa in conv["qa"]:
            yield qa["question"]


def embed(
    batch: list[str],
    tokenizer: transformers.PreTrainedTokenizerBase,
    model: transformers.PreTrainedModel,
) -> list[list[float]]:
    """Embed one batch of texts.

    Args:
        batch: The texts.
        tokenizer: The model's tokenizer.
        model: The sentence model.

    Returns:
        One vector per text, rounded to six decimals to keep the file small.
    """
    enc = tokenizer(batch, padding=True, truncation=True, return_tensors="pt")
    with torch.no_grad():
        hidden = model(**enc).last_hidden_state
    mask = enc["attention_mask"].unsqueeze(-1).float()
    pooled = (hidden * mask).sum(1) / mask.sum(1).clamp(min=1e-9)
    pooled = torch.nn.functional.normalize(pooled, p=2, dim=1)
    return [[round(x, 6) for x in vec] for vec in pooled.tolist()]


def main() -> None:
    """Embed the LoCoMo file named on the command line into the output file."""
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("data", type=Path)
    parser.add_argument("out", type=Path)
    args = parser.parse_args()

    unique = list(dict.fromkeys(texts(json.loads(args.data.read_text()))))
    tokenizer = transformers.AutoTokenizer.from_pretrained(MODEL)
    model = transformers.AutoModel.from_pretrained(MODEL)
    model.eval()
    vectors: dict[str, list[float]] = {}
    for start in range(0, len(unique), BATCH):
        batch = unique[start : start + BATCH]
        vectors.update(zip(batch, embed(batch, tokenizer, model), strict=True))
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(vectors, ensure_ascii=False))


if __name__ == "__main__":
    main()
