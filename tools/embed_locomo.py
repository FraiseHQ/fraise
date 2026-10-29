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

BenchmarkRetrievalQuality (pkg/server/locomo_bench_test.go) reads the result
through FRAISE_LOCOMO_VECTORS. It stops on any text it finds no vector for. So
the values built here must be the ones it builds, following the amb adapter's
raw path.

Each turn is one fact, valued ``[session N @ <date>] <speaker>: <text>``. Any
shared image's caption is appended to the text, and the ASCII apostrophe is
swapped for a typographic one. Each question is keyed as written.

The model is BAAI/bge-small-en-v1.5, a retrieval model of the same width as
the e2e suite's, 384, trained for questions against the passages that answer
them. A vector is its first token's hidden state, L2-normalized, the pooling
the model was trained with. A question is embedded behind the model's query
instruction, as the model expects, and keyed by the question alone. The
gates cache the file under a hash of the LoCoMo sample and
this script. So base and head read the same file, and are compared on
identical vectors, unless a pull request changes one of the two. That pull
request embeds anew, while the nightly baseline it is compared with used the
old vectors.

Run with the tests member's ``embeddings`` extra (``make perf-vectors``):

    python tools/embed_locomo.py <locomo.json> <vectors.json>
"""

import argparse
import json
from collections.abc import Iterator
from pathlib import Path

import torch
import transformers

MODEL = "BAAI/bge-small-en-v1.5"
QUERY = "Represent this sentence for searching relevant passages: "
BATCH = 64


def values(conversations: list[dict]) -> Iterator[str]:
    """Every value the benchmark stores, in order.

    Args:
        conversations: LoCoMo conversations, in the dataset's own format.

    Yields:
        The facts the benchmark needs a vector for.
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


def questions(conversations: list[dict]) -> Iterator[str]:
    """Every question the benchmark asks, in order.

    Args:
        conversations: LoCoMo conversations, in the dataset's own format.

    Yields:
        The questions the benchmark needs a vector for.
    """
    for conv in conversations:
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
    pooled = torch.nn.functional.normalize(hidden[:, 0], p=2, dim=1)
    return [[round(x, 6) for x in vec] for vec in pooled.tolist()]


def main() -> None:
    """Embed the LoCoMo file named on the command line into the output file."""
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("data", type=Path)
    parser.add_argument("out", type=Path)
    args = parser.parse_args()

    conversations = json.loads(args.data.read_text())
    tokenizer = transformers.AutoTokenizer.from_pretrained(MODEL)
    model = transformers.AutoModel.from_pretrained(MODEL)
    model.eval()
    vectors: dict[str, list[float]] = {}
    for texts, prefix in (
        (values(conversations), ""),
        (questions(conversations), QUERY),
    ):
        unique = list(dict.fromkeys(texts))
        for start in range(0, len(unique), BATCH):
            batch = unique[start : start + BATCH]
            embedded = embed([prefix + text for text in batch], tokenizer, model)
            vectors.update(zip(batch, embedded, strict=True))
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(vectors, ensure_ascii=False))


if __name__ == "__main__":
    main()
