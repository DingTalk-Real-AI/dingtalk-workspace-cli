#!/usr/bin/env python3
"""Evaluate DWS tool retrieval methods against reviewed Catalog metadata.

The primary intent benchmark deliberately excludes ``use_when``, ``avoid_when``,
and examples from the indexed corpus. The reviewed ``use_when`` entries become
queries, so a method cannot win by retrieving the exact sentence it indexed.

The script has no required third-party dependency. Pass ``--dense-model`` to
add a SentenceTransformers dense retriever and BM25F+dense RRF. For example::

    uv run --with fastembed==0.7.3 \
      scripts/dev/eval_tool_search_ranking.py \
      --dense-backend fastembed \
      --dense-model BAAI/bge-small-zh-v1.5 \
      --output /tmp/dws-tool-search-eval.json

The output is aggregate evidence, not a generated production baseline. Release
gates need a separately reviewed, versioned query/qrels dataset.
"""

from __future__ import annotations

import argparse
import json
import math
import random
import re
import statistics
import time
import unicodedata
from collections import Counter, defaultdict
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable, Protocol


REPO_ROOT = Path(__file__).resolve().parents[2]
DEFAULT_CATALOG = REPO_ROOT / ".worktrees/policy-tmp/tool-search-schema-catalog"
DEFAULT_WORKFLOWS = REPO_ROOT / "scripts/testdata/tool_search_workflows.json"
FIELD_WEIGHTS = {
    "identity": 8.0,
    "summary": 5.0,
    "description": 2.0,
    "parameters": 2.0,
}
DEFAULT_PROJECTION = "proxy_v1"
PROJECTION_SPECS = {
    "identity_only": {
        "identity": True,
    },
    "identity_summary": {
        "identity": True,
        "summary": True,
    },
    "identity_summary_description": {
        "identity": True,
        "summary": True,
        "description": True,
    },
    "proxy_v1": {
        "identity": True,
        "summary": True,
        "description": True,
        "parameter_names": True,
    },
    "with_parameter_descriptions": {
        "identity": True,
        "summary": True,
        "description": True,
        "parameter_names": True,
        "parameter_descriptions": True,
    },
    "production_v1_candidate": {
        "identity": True,
        "summary": True,
        "description": True,
        "parameter_names": True,
        "parameter_descriptions": True,
        "parameter_types": True,
        "aliases": True,
    },
    "production_with_use_when_leakage_upper_bound": {
        "identity": True,
        "summary": True,
        "description": True,
        "parameter_names": True,
        "parameter_descriptions": True,
        "parameter_types": True,
        "aliases": True,
        "use_when": True,
    },
}
CAMEL_BOUNDARY = re.compile(r"(?<=[a-z0-9])(?=[A-Z])")
IDENTIFIER = re.compile(r"[A-Za-z0-9]+(?:[._-][A-Za-z0-9]+)+")
TEXT_CHUNK = re.compile(r"[a-z0-9]+|[\u3400-\u9fff]+")


@dataclass(frozen=True)
class ToolDocument:
    canonical: str
    product: str
    cli_path: str
    identities: tuple[str, ...]
    fields: dict[str, str]
    use_when: tuple[str, ...]
    avoid_when: tuple[str, ...]
    examples: tuple[str, ...]

    @property
    def dense_text(self) -> str:
        return "\n".join(
            value for value in self.fields.values() if value.strip()
        )


@dataclass(frozen=True)
class SingleCase:
    query: str
    gold: str
    product: str
    kind: str = ""


@dataclass(frozen=True)
class WorkflowCase:
    case_id: str
    query: str
    required: tuple[str, ...]
    subqueries: tuple[str, ...]


class Ranker(Protocol):
    name: str

    def rank_many(
        self, queries: Iterable[str], limit: int | None = None
    ) -> dict[str, list[str]]: ...


def _strings(value: Any) -> list[str]:
    if isinstance(value, str):
        return [value]
    if isinstance(value, list):
        return [item for item in value if isinstance(item, str)]
    return []


def parameter_text(
    parameters: Any,
    *,
    include_names: bool,
    include_descriptions: bool,
    include_types: bool,
) -> str:
    if not isinstance(parameters, dict):
        return ""
    parts: list[str] = []
    for name in sorted(parameters):
        value = parameters[name]
        if not isinstance(value, dict):
            continue
        if include_names:
            parts.append(name)
            for key in ["property"]:
                item = value.get(key)
                if isinstance(item, str):
                    parts.append(item)
            enum = value.get("enum")
            if isinstance(enum, list):
                parts.extend(
                    str(item)
                    for item in enum
                    if isinstance(item, (str, int, float))
                )
        if include_descriptions:
            for key in ["description", "interface_description"]:
                item = value.get(key)
                if isinstance(item, str):
                    parts.append(item)
        if include_types:
            for key in ["type", "interface_type"]:
                item = value.get(key)
                if isinstance(item, str):
                    parts.append(item)
    return " ".join(parts)


def load_catalog(
    path: Path, projection: str = DEFAULT_PROJECTION
) -> tuple[str, list[ToolDocument]]:
    if projection not in PROJECTION_SPECS:
        raise ValueError(f"unknown projection {projection}")
    projection_spec = PROJECTION_SPECS[projection]
    payload = load_catalog_payload(path)
    tools: list[ToolDocument] = []
    for canonical, raw in sorted(payload["tools"].items()):
        identity_values = [
            str(raw.get(key, "")).strip()
            for key in [
                "canonical_path",
                "cli_path",
                "primary_cli_path",
                "name",
                "cli_name",
                "group",
                "product_id",
            ]
            if str(raw.get(key, "")).strip()
        ]
        if projection_spec.get("aliases"):
            identity_values.extend(_strings(raw.get("aliases")))
        identities = tuple(dict.fromkeys(identity_values))
        identity = " ".join(identities)
        summary = " ".join(
            str(raw.get(key, ""))
            for key in ["agent_summary", "title", "display"]
        )
        fields = {
            "identity": identity if projection_spec.get("identity") else "",
            "summary": summary if projection_spec.get("summary") else "",
            "description": (
                str(raw.get("description", ""))
                if projection_spec.get("description")
                else ""
            ),
            "parameters": parameter_text(
                raw.get("parameters"),
                include_names=bool(projection_spec.get("parameter_names")),
                include_descriptions=bool(
                    projection_spec.get("parameter_descriptions")
                ),
                include_types=bool(projection_spec.get("parameter_types")),
            ),
        }
        if projection_spec.get("use_when"):
            fields["use_when"] = " ".join(_strings(raw.get("use_when")))
        tools.append(
            ToolDocument(
                canonical=canonical,
                product=str(raw.get("product_id") or canonical.partition(".")[0]),
                cli_path=str(raw.get("cli_path", "")).strip(),
                identities=identities,
                fields=fields,
                use_when=tuple(_strings(raw.get("use_when"))),
                avoid_when=tuple(_strings(raw.get("avoid_when"))),
                examples=tuple(_strings(raw.get("examples"))),
            )
        )
    return str(payload.get("source_hash", "")), tools


def load_catalog_payload(path: Path) -> dict[str, Any]:
    """Load either a build-time Catalog shard directory or a test fixture.

    Production and CI use ``cmd_schema_catalog`` to materialize ``catalog.json``
    plus product shards below an ignored temporary directory. A consolidated
    JSON file remains accepted only so small unit fixtures and old diagnostic
    artifacts can be inspected without becoming repository delivery inputs.
    """
    if path.is_file():
        payload = json.loads(path.read_text(encoding="utf-8"))
        if not isinstance(payload.get("tools"), dict):
            raise ValueError(f"Catalog file has no tools object: {path}")
        return payload
    envelope_path = path / "catalog.json"
    tools_dir = path / "tools"
    if not envelope_path.is_file() or not tools_dir.is_dir():
        raise ValueError(
            "Catalog must be a generated directory containing catalog.json "
            f"and tools/: {path}"
        )
    envelope = json.loads(envelope_path.read_text(encoding="utf-8"))
    merged: dict[str, Any] = {}
    for shard_path in sorted(tools_dir.glob("*.json")):
        shard = json.loads(shard_path.read_text(encoding="utf-8"))
        shard_tools = shard.get("tools")
        if not isinstance(shard_tools, dict):
            raise ValueError(f"Catalog shard has no tools object: {shard_path}")
        duplicates = sorted(set(merged).intersection(shard_tools))
        if duplicates:
            raise ValueError(
                f"Catalog shards duplicate tools {duplicates}: {shard_path}"
            )
        merged.update(shard_tools)
    expected = envelope.get("catalog", {}).get("agent_metadata", {}).get(
        "surface_tools"
    )
    if isinstance(expected, int) and expected != len(merged):
        raise ValueError(
            f"Catalog shard count {len(merged)} differs from envelope {expected}"
        )
    return {
        "source_hash": envelope.get("source_hash", ""),
        "surface_hash": envelope.get("surface_hash", ""),
        "tools": merged,
    }


def load_workflows(path: Path, known_tools: set[str]) -> list[WorkflowCase]:
    payload = json.loads(path.read_text(encoding="utf-8"))
    cases: list[WorkflowCase] = []
    for raw in payload["cases"]:
        required = tuple(raw["required"])
        missing = sorted(set(required) - known_tools)
        if missing:
            raise ValueError(f"workflow {raw['id']} references missing tools: {missing}")
        cases.append(
            WorkflowCase(
                case_id=raw["id"],
                query=raw["query"],
                required=required,
                subqueries=tuple(raw["subqueries"]),
            )
        )
    return cases


def tokenize(text: str) -> list[str]:
    """Tokenize mixed Chinese, English, and CLI identifiers deterministically."""
    normalized = unicodedata.normalize("NFKC", text)
    tokens = [match.group(0).lower() for match in IDENTIFIER.finditer(normalized)]
    normalized = CAMEL_BOUNDARY.sub(" ", normalized).lower()
    normalized = normalized.replace("_", " ").replace("-", " ").replace(".", " ")
    for chunk in TEXT_CHUNK.findall(normalized):
        if "\u3400" <= chunk[0] <= "\u9fff":
            if len(chunk) == 1:
                tokens.append(chunk)
            else:
                tokens.extend(chunk[index : index + 2] for index in range(len(chunk) - 1))
        else:
            tokens.append(chunk)
    return tokens


def stable_rank(scores: dict[str, float], limit: int | None = None) -> list[str]:
    ranked = [item for item in scores.items() if item[1] > 0.0]
    ranked.sort(key=lambda item: (-item[1], item[0]))
    if limit is not None:
        ranked = ranked[:limit]
    return [canonical for canonical, _ in ranked]


class KeywordOverlapRanker:
    name = "keyword_overlap"

    def __init__(self, tools: list[ToolDocument]) -> None:
        self._tokens = {
            tool.canonical: set(
                tokenize(" ".join(tool.fields.values()))
            )
            for tool in tools
        }
        document_frequency: Counter[str] = Counter()
        for terms in self._tokens.values():
            document_frequency.update(terms)
        count = len(tools)
        self._idf = {
            term: math.log(1.0 + (count + 1.0) / (frequency + 1.0))
            for term, frequency in document_frequency.items()
        }

    def rank(self, query: str, limit: int | None = None) -> list[str]:
        terms = set(tokenize(query))
        scores = {
            canonical: math.fsum(
                self._idf.get(term, 0.0)
                for term in sorted(terms & document_terms)
            )
            for canonical, document_terms in self._tokens.items()
        }
        return stable_rank(scores, limit)

    def rank_many(
        self, queries: Iterable[str], limit: int | None = None
    ) -> dict[str, list[str]]:
        return {query: self.rank(query, limit) for query in dict.fromkeys(queries)}


class BM25Index:
    def __init__(
        self,
        documents: dict[str, list[str]],
        *,
        k1: float = 0.9,
        b: float = 0.4,
    ) -> None:
        self._documents = {key: Counter(tokens) for key, tokens in documents.items()}
        self._lengths = {key: sum(counts.values()) for key, counts in self._documents.items()}
        self._average_length = (
            statistics.fmean(self._lengths.values()) if self._lengths else 1.0
        )
        self._k1 = k1
        self._b = b
        self._count = len(documents)
        document_frequency: Counter[str] = Counter()
        for counts in self._documents.values():
            document_frequency.update(counts.keys())
        self._idf = {
            term: math.log(1.0 + (self._count - frequency + 0.5) / (frequency + 0.5))
            for term, frequency in document_frequency.items()
        }

    def scores(self, query: str) -> dict[str, float]:
        query_terms = Counter(tokenize(query))
        result: dict[str, float] = {}
        for canonical, counts in self._documents.items():
            length = max(self._lengths[canonical], 1)
            score = 0.0
            for term, query_frequency in query_terms.items():
                frequency = counts.get(term, 0)
                if frequency == 0:
                    continue
                denominator = frequency + self._k1 * (
                    1.0 - self._b + self._b * length / max(self._average_length, 1.0)
                )
                score += (
                    self._idf.get(term, 0.0)
                    * (frequency * (self._k1 + 1.0) / denominator)
                    * math.log1p(query_frequency)
                )
            result[canonical] = score
        return result


class TfidfCosineRanker:
    name = "tfidf_cosine"

    def __init__(self, tools: list[ToolDocument]) -> None:
        documents = {
            tool.canonical: Counter(tokenize(" ".join(tool.fields.values())))
            for tool in tools
        }
        document_frequency: Counter[str] = Counter()
        for counts in documents.values():
            document_frequency.update(counts.keys())
        count = len(documents)
        self._idf = {
            term: math.log((count + 1.0) / (frequency + 1.0)) + 1.0
            for term, frequency in document_frequency.items()
        }
        self._vectors: dict[str, dict[str, float]] = {}
        self._norms: dict[str, float] = {}
        for canonical, counts in documents.items():
            vector = {
                term: (1.0 + math.log(frequency)) * self._idf[term]
                for term, frequency in counts.items()
            }
            self._vectors[canonical] = vector
            self._norms[canonical] = math.sqrt(
                math.fsum(value * value for value in vector.values())
            )

    def rank(self, query: str, limit: int | None = None) -> list[str]:
        counts = Counter(tokenize(query))
        query_vector = {
            term: (1.0 + math.log(frequency)) * self._idf.get(term, 0.0)
            for term, frequency in counts.items()
            if term in self._idf
        }
        query_norm = math.sqrt(
            math.fsum(value * value for value in query_vector.values())
        )
        scores: dict[str, float] = {}
        if query_norm == 0:
            return []
        for canonical, vector in self._vectors.items():
            denominator = query_norm * self._norms[canonical]
            if denominator == 0:
                continue
            scores[canonical] = math.fsum(
                query_vector[term] * vector.get(term, 0.0)
                for term in sorted(query_vector)
            ) / denominator
        return stable_rank(scores, limit)

    def rank_many(
        self, queries: Iterable[str], limit: int | None = None
    ) -> dict[str, list[str]]:
        return {query: self.rank(query, limit) for query in dict.fromkeys(queries)}


class WeightedJaccardRanker:
    name = "weighted_jaccard"

    def __init__(self, tools: list[ToolDocument]) -> None:
        self._tokens = {
            tool.canonical: set(tokenize(" ".join(tool.fields.values())))
            for tool in tools
        }
        document_frequency: Counter[str] = Counter()
        for terms in self._tokens.values():
            document_frequency.update(terms)
        count = len(tools)
        self._idf = {
            term: math.log((count + 1.0) / (frequency + 1.0)) + 1.0
            for term, frequency in document_frequency.items()
        }

    def rank(self, query: str, limit: int | None = None) -> list[str]:
        query_terms = set(tokenize(query))
        scores: dict[str, float] = {}
        for canonical, document_terms in self._tokens.items():
            union = query_terms | document_terms
            denominator = math.fsum(
                self._idf.get(term, 0.0) for term in sorted(union)
            )
            if denominator == 0:
                continue
            numerator = math.fsum(
                self._idf.get(term, 0.0)
                for term in sorted(query_terms & document_terms)
            )
            scores[canonical] = numerator / denominator
        return stable_rank(scores, limit)

    def rank_many(
        self, queries: Iterable[str], limit: int | None = None
    ) -> dict[str, list[str]]:
        return {query: self.rank(query, limit) for query in dict.fromkeys(queries)}


class BM25VariantIndex(BM25Index):
    def __init__(
        self,
        documents: dict[str, list[str]],
        *,
        variant: str,
        k1: float = 0.9,
        b: float = 0.4,
        delta: float = 1.0,
    ) -> None:
        super().__init__(documents, k1=k1, b=b)
        self._variant = variant
        self._delta = delta

    def scores(self, query: str) -> dict[str, float]:
        query_terms = Counter(tokenize(query))
        result: dict[str, float] = {}
        for canonical, counts in self._documents.items():
            length = max(self._lengths[canonical], 1)
            length_ratio = length / max(self._average_length, 1.0)
            score = 0.0
            for term, query_frequency in query_terms.items():
                frequency = counts.get(term, 0)
                if frequency == 0:
                    continue
                if self._variant == "bm25l":
                    normalized = frequency / (1.0 - self._b + self._b * length_ratio)
                    contribution = (
                        (self._k1 + 1.0) * (normalized + self._delta)
                        / (self._k1 + normalized + self._delta)
                    )
                elif self._variant == "bm25plus":
                    denominator = frequency + self._k1 * (
                        1.0 - self._b + self._b * length_ratio
                    )
                    contribution = (
                        frequency * (self._k1 + 1.0) / denominator
                        + self._delta
                    )
                else:  # pragma: no cover - constructor-owned invariant
                    raise ValueError(f"unsupported BM25 variant {self._variant}")
                score += (
                    self._idf.get(term, 0.0)
                    * contribution
                    * math.log1p(query_frequency)
                )
            result[canonical] = score
        return result


class BM25LRanker:
    name = "bm25l"

    def __init__(self, tools: list[ToolDocument]) -> None:
        self._index = BM25VariantIndex(
            {
                tool.canonical: tokenize(" ".join(tool.fields.values()))
                for tool in tools
            },
            variant="bm25l",
        )

    def rank_many(
        self, queries: Iterable[str], limit: int | None = None
    ) -> dict[str, list[str]]:
        return {
            query: stable_rank(self._index.scores(query), limit)
            for query in dict.fromkeys(queries)
        }


class BM25PlusRanker:
    name = "bm25plus"

    def __init__(self, tools: list[ToolDocument]) -> None:
        self._index = BM25VariantIndex(
            {
                tool.canonical: tokenize(" ".join(tool.fields.values()))
                for tool in tools
            },
            variant="bm25plus",
        )

    def rank_many(
        self, queries: Iterable[str], limit: int | None = None
    ) -> dict[str, list[str]]:
        return {
            query: stable_rank(self._index.scores(query), limit)
            for query in dict.fromkeys(queries)
        }


class BM25Ranker:
    name = "bm25_unfielded"

    def __init__(self, tools: list[ToolDocument]) -> None:
        self._index = BM25Index(
            {
                tool.canonical: tokenize(" ".join(tool.fields.values()))
                for tool in tools
            }
        )

    def rank(self, query: str, limit: int | None = None) -> list[str]:
        return stable_rank(self._index.scores(query), limit)

    def rank_many(
        self, queries: Iterable[str], limit: int | None = None
    ) -> dict[str, list[str]]:
        return {query: self.rank(query, limit) for query in dict.fromkeys(queries)}


class BM25FRanker:
    name = "bm25_field_weighted"

    def __init__(self, tools: list[ToolDocument]) -> None:
        self._indexes = {
            field: BM25Index(
                {tool.canonical: tokenize(tool.fields[field]) for tool in tools}
            )
            for field in FIELD_WEIGHTS
        }
        self._identities = {
            tool.canonical: {
                unicodedata.normalize("NFKC", value).strip().lower()
                for value in tool.identities
                if value.strip()
            }
            for tool in tools
        }
        self._tools = tools

    def rank(self, query: str, limit: int | None = None) -> list[str]:
        scores: defaultdict[str, float] = defaultdict(float)
        for field, index in self._indexes.items():
            weight = FIELD_WEIGHTS[field]
            for canonical, score in index.scores(query).items():
                scores[canonical] += weight * score
        normalized_query = unicodedata.normalize("NFKC", query).strip().lower()
        for tool in self._tools:
            canonical = tool.canonical
            identity_values = self._identities[canonical]
            if normalized_query == canonical.lower():
                scores[canonical] += 1_000_000.0
            elif any(normalized_query == value for value in identity_values):
                scores[canonical] += 100_000.0
        return stable_rank(scores, limit)

    def rank_many(
        self, queries: Iterable[str], limit: int | None = None
    ) -> dict[str, list[str]]:
        return {query: self.rank(query, limit) for query in dict.fromkeys(queries)}


class SentenceTransformerDenseRanker:
    name = "dense"

    def __init__(self, tools: list[ToolDocument], model_name: str, batch_size: int) -> None:
        try:
            import numpy as np
            from sentence_transformers import SentenceTransformer
        except ImportError as exc:  # pragma: no cover - environment dependent
            raise RuntimeError(
                "--dense-model requires sentence-transformers; use the uv command "
                "shown in this script's docstring"
            ) from exc
        self._np = np
        self._model_name = model_name
        self._model = SentenceTransformer(model_name)
        self._batch_size = batch_size
        self._canonicals = [tool.canonical for tool in tools]
        documents = [self._document_input(tool.dense_text) for tool in tools]
        self._document_vectors = self._model.encode(
            documents,
            batch_size=batch_size,
            normalize_embeddings=True,
            show_progress_bar=True,
            convert_to_numpy=True,
        )

    def _query_input(self, text: str) -> str:
        if "e5" in self._model_name.lower():
            return f"query: {text}"
        return text

    def _document_input(self, text: str) -> str:
        if "e5" in self._model_name.lower():
            return f"passage: {text}"
        return text

    def rank_many(
        self, queries: Iterable[str], limit: int | None = None
    ) -> dict[str, list[str]]:
        unique_queries = list(dict.fromkeys(queries))
        vectors = self._model.encode(
            [self._query_input(query) for query in unique_queries],
            batch_size=self._batch_size,
            normalize_embeddings=True,
            show_progress_bar=True,
            convert_to_numpy=True,
        )
        similarities = vectors @ self._document_vectors.T
        result: dict[str, list[str]] = {}
        for row, query in zip(similarities, unique_queries):
            order = self._np.lexsort(
                (
                    self._np.asarray(self._canonicals, dtype=object),
                    -row,
                )
            )
            if limit is not None:
                order = order[:limit]
            result[query] = [self._canonicals[int(index)] for index in order]
        return result


class FastEmbedDenseRanker:
    name = "dense"

    def __init__(self, tools: list[ToolDocument], model_name: str, batch_size: int) -> None:
        try:
            import numpy as np
            from fastembed import TextEmbedding
        except ImportError as exc:  # pragma: no cover - environment dependent
            raise RuntimeError(
                "--dense-backend fastembed requires fastembed; use the uv command "
                "shown in this script's docstring"
            ) from exc
        self._np = np
        self._model = TextEmbedding(model_name=model_name)
        self._batch_size = batch_size
        self._canonicals = [tool.canonical for tool in tools]
        document_vectors = list(
            self._model.passage_embed(
                [tool.dense_text for tool in tools], batch_size=batch_size
            )
        )
        self._document_vectors = self._normalize(np.asarray(document_vectors))

    def _normalize(self, vectors: Any) -> Any:
        norms = self._np.linalg.norm(vectors, axis=1, keepdims=True)
        return vectors / self._np.maximum(norms, 1e-12)

    def rank_many(
        self, queries: Iterable[str], limit: int | None = None
    ) -> dict[str, list[str]]:
        unique_queries = list(dict.fromkeys(queries))
        query_vectors = list(
            self._model.query_embed(unique_queries, batch_size=self._batch_size)
        )
        vectors = self._normalize(self._np.asarray(query_vectors))
        similarities = vectors @ self._document_vectors.T
        result: dict[str, list[str]] = {}
        for row, query in zip(similarities, unique_queries):
            order = self._np.lexsort(
                (
                    self._np.asarray(self._canonicals, dtype=object),
                    -row,
                )
            )
            if limit is not None:
                order = order[:limit]
            result[query] = [self._canonicals[int(index)] for index in order]
        return result


def rrf_rank(
    rankings: Iterable[list[str]], limit: int | None = None, k: float = 60.0
) -> list[str]:
    scores: defaultdict[str, float] = defaultdict(float)
    for ranking in rankings:
        for rank, canonical in enumerate(ranking, start=1):
            scores[canonical] += 1.0 / (k + rank)
    return stable_rank(scores, limit)


def reciprocal_rank(ranking: list[str], gold: str) -> float:
    try:
        return 1.0 / (ranking.index(gold) + 1)
    except ValueError:
        return 0.0


def rank_position(ranking: list[str], gold: str) -> int | None:
    try:
        return ranking.index(gold) + 1
    except ValueError:
        return None


def evaluate_single_cases(
    cases: list[SingleCase], rankings: dict[str, list[str]]
) -> tuple[dict[str, Any], list[bool]]:
    reciprocal_ranks: list[float] = []
    positions: list[int] = []
    top1: list[bool] = []
    top5: list[bool] = []
    top10: list[bool] = []
    empty = 0
    per_product: defaultdict[str, list[bool]] = defaultdict(list)
    for case in cases:
        ranking = rankings[case.query]
        if not ranking:
            empty += 1
        position = rank_position(ranking, case.gold)
        if position is not None:
            positions.append(position)
        reciprocal_ranks.append(reciprocal_rank(ranking, case.gold))
        hit1 = position is not None and position <= 1
        hit5 = position is not None and position <= 5
        hit10 = position is not None and position <= 10
        top1.append(hit1)
        top5.append(hit5)
        top10.append(hit10)
        per_product[case.product].append(hit5)
    product_recall = {
        product: sum(values) / len(values)
        for product, values in sorted(per_product.items())
    }
    metrics: dict[str, Any] = {
        "cases": len(cases),
        "recall_at_1": sum(top1) / len(cases),
        "recall_at_5": sum(top5) / len(cases),
        "recall_at_10": sum(top10) / len(cases),
        "mrr": statistics.fmean(reciprocal_ranks),
        "median_rank_when_found": statistics.median(positions) if positions else None,
        "empty_result_rate": empty / len(cases),
        "worst_product_recall_at_5": dict(
            sorted(product_recall.items(), key=lambda item: (item[1], item[0]))[:5]
        ),
        "product_recall_at_5": product_recall,
    }
    return metrics, top5


def evaluate_forbidden(
    cases: list[SingleCase], rankings: dict[str, list[str]]
) -> dict[str, Any]:
    ranks = [rank_position(rankings[case.query], case.gold) for case in cases]
    return {
        "cases": len(cases),
        "forbidden_at_1": sum(rank == 1 for rank in ranks) / len(cases),
        "forbidden_at_5": sum(rank is not None and rank <= 5 for rank in ranks)
        / len(cases),
        "forbidden_at_10": sum(rank is not None and rank <= 10 for rank in ranks)
        / len(cases),
    }


def evaluate_workflows(
    cases: list[WorkflowCase], rankings: dict[str, list[str]], top_k: int = 5
) -> dict[str, Any]:
    complete = 0
    recalls: list[float] = []
    ndcgs: list[float] = []
    details: list[dict[str, Any]] = []
    for case in cases:
        selected = rankings[case.query][:top_k]
        found = [tool for tool in case.required if tool in selected]
        missing = [tool for tool in case.required if tool not in selected]
        complete += not missing
        recalls.append(len(found) / len(case.required))
        dcg = math.fsum(
            1.0 / math.log2(rank + 2.0)
            for rank, canonical in enumerate(selected)
            if canonical in case.required
        )
        ideal_count = min(len(case.required), top_k)
        ideal_dcg = math.fsum(
            1.0 / math.log2(rank + 2.0) for rank in range(ideal_count)
        )
        ndcgs.append(dcg / ideal_dcg if ideal_dcg else 0.0)
        details.append(
            {
                "id": case.case_id,
                "required": list(case.required),
                "top_k": selected,
                "missing": missing,
            }
        )
    return {
        "cases": len(cases),
        "comprehensiveness_at_5": complete / len(cases),
        "mean_required_recall_at_5": statistics.fmean(recalls),
        "mean_ndcg_at_5": statistics.fmean(ndcgs),
        "details": details,
    }


def decompose_workflow_rankings(
    cases: list[WorkflowCase],
    rankings: dict[str, list[str]],
    *,
    top_k: int = 5,
) -> dict[str, list[str]]:
    """Round-robin per-step rankings for a manually reviewed upper bound."""
    result: dict[str, list[str]] = {}
    for case in cases:
        selected: list[str] = []
        depth = 0
        while len(selected) < top_k:
            added = False
            for subquery in case.subqueries:
                ranking = rankings[subquery]
                if depth >= len(ranking):
                    continue
                canonical = ranking[depth]
                if canonical not in selected:
                    selected.append(canonical)
                    added = True
                    if len(selected) == top_k:
                        break
            if not added:
                break
            depth += 1
        result[case.query] = selected
    return result


def diagnostic_examples(
    cases: list[SingleCase], rankings: dict[str, list[str]], *, forbidden: bool = False
) -> list[dict[str, Any]]:
    examples: list[dict[str, Any]] = []
    for case in cases:
        top5 = rankings[case.query][:5]
        position = rank_position(rankings[case.query], case.gold)
        selected = position == 1 if forbidden else position is None or position > 5
        if not selected:
            continue
        examples.append(
            {
                "query": case.query,
                "gold" if not forbidden else "forbidden": case.gold,
                "top_5": top5,
            }
        )
        if len(examples) == 8:
            break
    return examples


def paired_change_examples(
    cases: list[SingleCase],
    baseline: dict[str, list[str]],
    candidate: dict[str, list[str]],
) -> dict[str, list[dict[str, Any]]]:
    rescued: list[dict[str, Any]] = []
    regressed: list[dict[str, Any]] = []
    for case in cases:
        base_hit = case.gold in baseline[case.query][:5]
        candidate_hit = case.gold in candidate[case.query][:5]
        if base_hit == candidate_hit:
            continue
        record = {
            "query": case.query,
            "gold": case.gold,
            "baseline_top_5": baseline[case.query][:5],
            "candidate_top_5": candidate[case.query][:5],
        }
        target = rescued if candidate_hit else regressed
        if len(target) < 8:
            target.append(record)
    return {"rescued": rescued, "regressed": regressed}


def paired_bootstrap_interval(
    baseline: list[bool], candidate: list[bool], samples: int = 2_000
) -> dict[str, float]:
    if len(baseline) != len(candidate):
        raise ValueError("paired samples differ in length")
    random_source = random.Random(20260812)
    differences: list[float] = []
    count = len(baseline)
    observed = statistics.fmean(candidate) - statistics.fmean(baseline)
    for _ in range(samples):
        indexes = [random_source.randrange(count) for _ in range(count)]
        difference = statistics.fmean(candidate[index] for index in indexes) - statistics.fmean(
            baseline[index] for index in indexes
        )
        differences.append(difference)
    differences.sort()
    return {
        "recall_at_5_delta": observed,
        "bootstrap_95_low": differences[int(samples * 0.025)],
        "bootstrap_95_high": differences[int(samples * 0.975)],
    }


def paired_product_cluster_bootstrap_interval(
    cases: list[SingleCase],
    baseline: list[bool],
    candidate: list[bool],
    samples: int = 2_000,
) -> dict[str, float]:
    """Bootstrap macro-product paired R@5 differences.

    Cases written for the same product share authors, vocabulary, and sibling
    tools, so treating all rows as independent overstates cross-product
    certainty. Each draw resamples product clusters and averages their paired
    mean differences with equal product weight.
    """
    if len(cases) != len(baseline) or len(baseline) != len(candidate):
        raise ValueError("clustered paired samples differ in length")
    clusters: defaultdict[str, list[int]] = defaultdict(list)
    for index, case in enumerate(cases):
        clusters[case.product].append(index)
    products = sorted(clusters)
    if not products:
        raise ValueError("clustered paired samples are empty")
    product_differences = {
        product: statistics.fmean(
            float(candidate[index]) - float(baseline[index])
            for index in clusters[product]
        )
        for product in products
    }
    observed = statistics.fmean(product_differences.values())
    random_source = random.Random(20260812)
    differences: list[float] = []
    for _ in range(samples):
        selected = [
            products[random_source.randrange(len(products))]
            for _ in range(len(products))
        ]
        differences.append(
            statistics.fmean(product_differences[product] for product in selected)
        )
    differences.sort()
    return {
        "macro_product_recall_at_5_delta": observed,
        "bootstrap_95_low": differences[int(samples * 0.025)],
        "bootstrap_95_high": differences[int(samples * 0.975)],
        "product_clusters": len(products),
    }


def query_language_slice(query: str) -> str:
    has_cjk = any("\u3400" <= character <= "\u9fff" for character in query)
    has_ascii_word = any(character.isascii() and character.isalnum() for character in query)
    if has_cjk and has_ascii_word:
        return "mixed_chinese_ascii"
    if has_cjk:
        return "chinese_only"
    return "non_chinese"


def evaluate_language_slices(
    cases: list[SingleCase], rankings: dict[str, list[str]]
) -> dict[str, dict[str, Any]]:
    result: dict[str, dict[str, Any]] = {}
    for slice_name in ["chinese_only", "mixed_chinese_ascii", "non_chinese"]:
        selected = [
            case for case in cases if query_language_slice(case.query) == slice_name
        ]
        if selected:
            result[slice_name] = evaluate_single_cases(selected, rankings)[0]
    return result


def percentile(values: list[float], fraction: float) -> float:
    if not values:
        raise ValueError("percentile input is empty")
    ordered = sorted(values)
    index = max(0, min(len(ordered) - 1, math.ceil(fraction * len(ordered)) - 1))
    return ordered[index]


def measure_single_query_latency(
    ranker: Ranker,
    queries: list[str],
    *,
    samples: int,
) -> dict[str, float | int]:
    """Measure warmed one-query Python latency without claiming production SLA."""
    if samples <= 0:
        return {}
    unique_queries = list(dict.fromkeys(queries))
    if not unique_queries:
        raise ValueError("latency query set is empty")
    warmup = min(20, samples)
    for index in range(warmup):
        ranker.rank_many([unique_queries[index % len(unique_queries)]], limit=5)
    durations: list[float] = []
    for index in range(samples):
        query = unique_queries[index % len(unique_queries)]
        started = time.perf_counter_ns()
        ranker.rank_many([query], limit=5)
        durations.append((time.perf_counter_ns() - started) / 1_000_000.0)
    return {
        "samples": samples,
        "p50": statistics.median(durations),
        "p95": percentile(durations, 0.95),
        "p99": percentile(durations, 0.99),
        "max": max(durations),
    }


def evaluate_projection_ablation(
    catalog_path: Path,
    intent: list[SingleCase],
    forbidden: list[SingleCase],
    workflows: list[WorkflowCase],
    all_queries: list[str],
) -> dict[str, Any]:
    """Compare cumulative field projections with identical ranker contracts."""
    result: dict[str, Any] = {
        "order": list(PROJECTION_SPECS),
        "projections": {},
        "paired_vs_proxy_v1": {},
        "paired_product_cluster_vs_proxy_v1": {},
    }
    success_vectors: dict[tuple[str, str], list[bool]] = {}
    for projection_name, projection_spec in PROJECTION_SPECS.items():
        _, tools = load_catalog(catalog_path, projection_name)
        projection_result: dict[str, Any] = {
            "fields": sorted(
                key for key, enabled in projection_spec.items() if enabled
            ),
            "methods": {},
        }
        for ranker_type in [TfidfCosineRanker, BM25Ranker]:
            started = time.perf_counter()
            ranker = ranker_type(tools)
            build_seconds = time.perf_counter() - started
            started = time.perf_counter()
            rankings = ranker.rank_many(all_queries)
            batch_seconds = time.perf_counter() - started
            intent_metrics, success = evaluate_single_cases(intent, rankings)
            success_vectors[(projection_name, ranker.name)] = success
            projection_result["methods"][ranker.name] = {
                "index_build_seconds": build_seconds,
                "offline_batch_seconds": batch_seconds,
                "intent": intent_metrics,
                "intent_language_slices": evaluate_language_slices(
                    intent, rankings
                ),
                "forbidden": evaluate_forbidden(forbidden, rankings),
                "workflow": evaluate_workflows(workflows, rankings),
            }
        result["projections"][projection_name] = projection_result

    for projection_name in PROJECTION_SPECS:
        if projection_name == DEFAULT_PROJECTION:
            continue
        result["paired_vs_proxy_v1"][projection_name] = {}
        result["paired_product_cluster_vs_proxy_v1"][projection_name] = {}
        for method in ["tfidf_cosine", "bm25_unfielded"]:
            baseline = success_vectors[(DEFAULT_PROJECTION, method)]
            candidate = success_vectors[(projection_name, method)]
            result["paired_vs_proxy_v1"][projection_name][method] = (
                paired_bootstrap_interval(baseline, candidate)
            )
            result["paired_product_cluster_vs_proxy_v1"][projection_name][method] = (
                paired_product_cluster_bootstrap_interval(
                    intent, baseline, candidate
                )
            )
    return result


def build_cases(
    tools: list[ToolDocument], workflows_path: Path
) -> tuple[list[SingleCase], list[SingleCase], list[SingleCase], list[WorkflowCase]]:
    intent: list[SingleCase] = []
    forbidden: list[SingleCase] = []
    identity: list[SingleCase] = []
    for tool in tools:
        intent.extend(
            SingleCase(query=query, gold=tool.canonical, product=tool.product)
            for query in tool.use_when
        )
        forbidden.extend(
            SingleCase(query=query, gold=tool.canonical, product=tool.product)
            for query in tool.avoid_when
        )
        identity.append(
            SingleCase(
                query=tool.canonical,
                gold=tool.canonical,
                product=tool.product,
                kind="canonical",
            )
        )
        if tool.cli_path:
            identity.append(
                SingleCase(
                    query=tool.cli_path,
                    gold=tool.canonical,
                    product=tool.product,
                    kind="cli_path",
                )
            )
    workflows = load_workflows(workflows_path, {tool.canonical for tool in tools})
    return intent, forbidden, identity, workflows


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--catalog",
        type=Path,
        default=DEFAULT_CATALOG,
        help=(
            "build-time Catalog shard directory from cmd_schema_catalog; "
            "a consolidated JSON file is accepted for diagnostic fixtures"
        ),
    )
    parser.add_argument("--workflows", type=Path, default=DEFAULT_WORKFLOWS)
    parser.add_argument("--dense-model", default="")
    parser.add_argument(
        "--dense-backend",
        choices=["fastembed", "sentence-transformers"],
        default="fastembed",
    )
    parser.add_argument("--batch-size", type=int, default=64)
    parser.add_argument("--rrf-depth", type=int, default=100)
    parser.add_argument(
        "--projection",
        choices=list(PROJECTION_SPECS),
        default=DEFAULT_PROJECTION,
    )
    parser.add_argument(
        "--skip-projection-ablation",
        action="store_true",
        help="skip cumulative field projection comparisons",
    )
    parser.add_argument(
        "--latency-samples",
        type=int,
        default=0,
        help="measure warmed Python one-query latency for each base ranker",
    )
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()

    source_hash, tools = load_catalog(args.catalog, args.projection)
    intent, forbidden, identity, workflows = build_cases(tools, args.workflows)
    all_queries = [
        case.query for case in [*intent, *forbidden, *identity]
    ] + [
        query
        for case in workflows
        for query in (case.query, *case.subqueries)
    ]

    rankers: list[Ranker] = []
    build_seconds: dict[str, float] = {}
    for ranker_type in [
        KeywordOverlapRanker,
        TfidfCosineRanker,
        WeightedJaccardRanker,
        BM25Ranker,
        BM25LRanker,
        BM25PlusRanker,
        BM25FRanker,
    ]:
        started = time.perf_counter()
        ranker = ranker_type(tools)
        build_seconds[ranker.name] = time.perf_counter() - started
        rankers.append(ranker)
    if args.dense_model:
        started = time.perf_counter()
        if args.dense_backend == "fastembed":
            dense_ranker: Ranker = FastEmbedDenseRanker(
                tools, args.dense_model, args.batch_size
            )
        else:
            dense_ranker = SentenceTransformerDenseRanker(
                tools, args.dense_model, args.batch_size
            )
        build_seconds[dense_ranker.name] = time.perf_counter() - started
        rankers.append(dense_ranker)

    result: dict[str, Any] = {
        "protocol": {
            "catalog": str(args.catalog),
            "catalog_source_hash": source_hash,
            "tool_count": len(tools),
            "intent_cases": len(intent),
            "forbidden_cases": len(forbidden),
            "identity_cases": len(identity),
            "workflow_cases": len(workflows),
            "index_excludes": [
                field
                for field in ["use_when", "avoid_when", "examples"]
                if field != "use_when"
                or not PROJECTION_SPECS[args.projection].get("use_when")
            ],
            "projection_version": args.projection,
            "projection_fields": sorted(
                key
                for key, enabled in PROJECTION_SPECS[args.projection].items()
                if enabled
            ),
            "intent_qrels_limitation": (
                "use_when queries and indexed agent_summary text are authored in "
                "the same reviewed selection files; this is a proxy benchmark, "
                "not an independent release test set"
            ),
            "field_weights": FIELD_WEIGHTS,
            "dense_model": args.dense_model or None,
            "dense_backend": args.dense_backend if args.dense_model else None,
            "rrf_depth": args.rrf_depth,
        },
        "methods": {},
        "paired_vs_bm25_field_weighted": {},
        "paired_product_cluster_vs_bm25_field_weighted": {},
        "paired_vs_bm25_unfielded": {},
        "paired_product_cluster_vs_bm25_unfielded": {},
    }
    all_rankings: dict[str, dict[str, list[str]]] = {}
    success_vectors: dict[str, list[bool]] = {}
    for ranker in rankers:
        started = time.perf_counter()
        rankings = ranker.rank_many(all_queries)
        duration = time.perf_counter() - started
        all_rankings[ranker.name] = rankings
        intent_metrics, success = evaluate_single_cases(intent, rankings)
        success_vectors[ranker.name] = success
        identity_metrics, _ = evaluate_single_cases(identity, rankings)
        identity_by_kind = {
            kind: evaluate_single_cases(
                [case for case in identity if case.kind == kind], rankings
            )[0]
            for kind in ["canonical", "cli_path"]
        }
        result["methods"][ranker.name] = {
            "index_build_seconds": build_seconds[ranker.name],
            "offline_batch_seconds": duration,
            "single_query_latency_ms": measure_single_query_latency(
                ranker,
                [case.query for case in intent],
                samples=args.latency_samples,
            ),
            "intent": intent_metrics,
            "intent_language_slices": evaluate_language_slices(intent, rankings),
            "forbidden": evaluate_forbidden(forbidden, rankings),
            "identity": identity_metrics,
            "identity_by_kind": identity_by_kind,
            "workflow": evaluate_workflows(workflows, rankings),
            "diagnostics": {
                "intent_misses": diagnostic_examples(intent, rankings),
                "forbidden_at_1": diagnostic_examples(
                    forbidden, rankings, forbidden=True
                ),
            },
        }

    result["lexical_rrf_sweep"] = []
    for depth in [20, 50, 100]:
        for rrf_k in [10.0, 20.0, 60.0]:
            lexical_rankings = {
                query: rrf_rank(
                    [
                        all_rankings["tfidf_cosine"][query][:depth],
                        all_rankings["bm25_unfielded"][query][:depth],
                    ],
                    k=rrf_k,
                )
                for query in dict.fromkeys(all_queries)
            }
            metrics, _ = evaluate_single_cases(intent, lexical_rankings)
            result["lexical_rrf_sweep"].append(
                {
                    "arms": ["tfidf_cosine", "bm25_unfielded"],
                    "depth": depth,
                    "k": rrf_k,
                    "intent": metrics,
                    "forbidden": evaluate_forbidden(forbidden, lexical_rankings),
                    "workflow": evaluate_workflows(workflows, lexical_rankings),
                }
            )

    lexical_hybrid_name = "lexical_rrf_tfidf_bm25"
    lexical_hybrid_rankings = {
        query: rrf_rank(
            [
                all_rankings["tfidf_cosine"][query][: args.rrf_depth],
                all_rankings["bm25_unfielded"][query][: args.rrf_depth],
            ]
        )
        for query in dict.fromkeys(all_queries)
    }
    all_rankings[lexical_hybrid_name] = lexical_hybrid_rankings
    intent_metrics, success = evaluate_single_cases(intent, lexical_hybrid_rankings)
    success_vectors[lexical_hybrid_name] = success
    result["methods"][lexical_hybrid_name] = {
        "index_build_seconds": None,
        "offline_batch_seconds": None,
        "single_query_latency_ms": {},
        "intent": intent_metrics,
        "intent_language_slices": evaluate_language_slices(
            intent, lexical_hybrid_rankings
        ),
        "forbidden": evaluate_forbidden(forbidden, lexical_hybrid_rankings),
        "identity": evaluate_single_cases(identity, lexical_hybrid_rankings)[0],
        "identity_by_kind": {
            kind: evaluate_single_cases(
                [case for case in identity if case.kind == kind],
                lexical_hybrid_rankings,
            )[0]
            for kind in ["canonical", "cli_path"]
        },
        "workflow": evaluate_workflows(workflows, lexical_hybrid_rankings),
        "diagnostics": {
            "intent_misses": diagnostic_examples(intent, lexical_hybrid_rankings),
            "forbidden_at_1": diagnostic_examples(
                forbidden, lexical_hybrid_rankings, forbidden=True
            ),
        },
    }

    if "dense" in all_rankings:
        dense_ranks = all_rankings["dense"]
        result["dense_rrf_sweep"] = []
        for sparse_name in [
            "tfidf_cosine",
            "bm25_unfielded",
            "bm25_field_weighted",
        ]:
            sparse_ranks = all_rankings[sparse_name]
            for depth in [20, 50, 100]:
                for rrf_k in [10.0, 20.0, 60.0]:
                    sweep_rankings = {
                        query: rrf_rank(
                            [
                                sparse_ranks[query][:depth],
                                dense_ranks[query][:depth],
                            ],
                            k=rrf_k,
                        )
                        for query in dict.fromkeys(all_queries)
                    }
                    sweep_metrics, _ = evaluate_single_cases(
                        intent, sweep_rankings
                    )
                    result["dense_rrf_sweep"].append(
                        {
                            "arms": [sparse_name, "dense"],
                            "depth": depth,
                            "k": rrf_k,
                            "intent": sweep_metrics,
                            "forbidden": evaluate_forbidden(
                                forbidden, sweep_rankings
                            ),
                            "workflow": evaluate_workflows(
                                workflows, sweep_rankings
                            ),
                        }
                    )
        for hybrid_name, sparse_name in [
            ("hybrid_rrf_tfidf", "tfidf_cosine"),
            ("hybrid_rrf_fielded", "bm25_field_weighted"),
            ("hybrid_rrf_unfielded", "bm25_unfielded"),
        ]:
            sparse_ranks = all_rankings[sparse_name]
            hybrid_rankings = {
                query: rrf_rank(
                    [
                        sparse_ranks[query][: args.rrf_depth],
                        dense_ranks[query][: args.rrf_depth],
                    ]
                )
                for query in dict.fromkeys(all_queries)
            }
            all_rankings[hybrid_name] = hybrid_rankings
            intent_metrics, success = evaluate_single_cases(intent, hybrid_rankings)
            success_vectors[hybrid_name] = success
            identity_metrics, _ = evaluate_single_cases(identity, hybrid_rankings)
            identity_by_kind = {
                kind: evaluate_single_cases(
                    [case for case in identity if case.kind == kind], hybrid_rankings
                )[0]
                for kind in ["canonical", "cli_path"]
            }
            result["methods"][hybrid_name] = {
                "index_build_seconds": None,
                "offline_batch_seconds": None,
                "intent": intent_metrics,
                "intent_language_slices": evaluate_language_slices(intent, hybrid_rankings),
                "forbidden": evaluate_forbidden(forbidden, hybrid_rankings),
                "identity": identity_metrics,
                "identity_by_kind": identity_by_kind,
                "workflow": evaluate_workflows(workflows, hybrid_rankings),
                "diagnostics": {
                    "intent_misses": diagnostic_examples(intent, hybrid_rankings),
                    "forbidden_at_1": diagnostic_examples(
                        forbidden, hybrid_rankings, forbidden=True
                    ),
                },
            }

        exact_gold = {case.query: case.gold for case in identity}
        fielded_hybrid = all_rankings["hybrid_rrf_fielded"]
        guarded_name = "hybrid_rrf_fielded_exact_guard"
        guarded_rankings = {
            query: [exact_gold[query]]
            if query in exact_gold
            else fielded_hybrid[query]
            for query in dict.fromkeys(all_queries)
        }
        all_rankings[guarded_name] = guarded_rankings
        intent_metrics, success = evaluate_single_cases(intent, guarded_rankings)
        success_vectors[guarded_name] = success
        identity_metrics, _ = evaluate_single_cases(identity, guarded_rankings)
        identity_by_kind = {
            kind: evaluate_single_cases(
                [case for case in identity if case.kind == kind], guarded_rankings
            )[0]
            for kind in ["canonical", "cli_path"]
        }
        result["methods"][guarded_name] = {
            "index_build_seconds": None,
            "offline_batch_seconds": None,
            "intent": intent_metrics,
            "intent_language_slices": evaluate_language_slices(intent, guarded_rankings),
            "forbidden": evaluate_forbidden(forbidden, guarded_rankings),
            "identity": identity_metrics,
            "identity_by_kind": identity_by_kind,
            "workflow": evaluate_workflows(workflows, guarded_rankings),
            "diagnostics": {
                "intent_misses": diagnostic_examples(intent, guarded_rankings),
                "forbidden_at_1": diagnostic_examples(
                    forbidden, guarded_rankings, forbidden=True
                ),
            },
        }

        tfidf_hybrid = all_rankings["hybrid_rrf_tfidf"]
        tfidf_guarded_name = "hybrid_rrf_tfidf_exact_guard"
        tfidf_guarded_rankings = {
            query: [exact_gold[query]]
            if query in exact_gold
            else tfidf_hybrid[query]
            for query in dict.fromkeys(all_queries)
        }
        all_rankings[tfidf_guarded_name] = tfidf_guarded_rankings
        intent_metrics, success = evaluate_single_cases(
            intent, tfidf_guarded_rankings
        )
        success_vectors[tfidf_guarded_name] = success
        result["methods"][tfidf_guarded_name] = {
            "index_build_seconds": None,
            "offline_batch_seconds": None,
            "intent": intent_metrics,
            "intent_language_slices": evaluate_language_slices(
                intent, tfidf_guarded_rankings
            ),
            "forbidden": evaluate_forbidden(forbidden, tfidf_guarded_rankings),
            "identity": evaluate_single_cases(identity, tfidf_guarded_rankings)[0],
            "identity_by_kind": {
                kind: evaluate_single_cases(
                    [case for case in identity if case.kind == kind],
                    tfidf_guarded_rankings,
                )[0]
                for kind in ["canonical", "cli_path"]
            },
            "workflow": evaluate_workflows(workflows, tfidf_guarded_rankings),
            "diagnostics": {
                "intent_misses": diagnostic_examples(
                    intent, tfidf_guarded_rankings
                ),
                "forbidden_at_1": diagnostic_examples(
                    forbidden, tfidf_guarded_rankings, forbidden=True
                ),
            },
        }

        result["comparison_examples"] = paired_change_examples(
            intent,
            all_rankings["bm25_field_weighted"],
            all_rankings["hybrid_rrf_fielded"],
        )
        decomposed_rankings = decompose_workflow_rankings(
            workflows, all_rankings[guarded_name]
        )
        result["workflow_decomposition_upper_bound"] = {
            "method": "manual reviewed subqueries + fielded hybrid RRF + exact guard",
            "warning": "Subqueries are human-authored; this measures retrieval after correct decomposition, not automatic planner accuracy.",
            "workflow": evaluate_workflows(workflows, decomposed_rankings),
        }

    baseline = success_vectors["bm25_field_weighted"]
    for name, success in success_vectors.items():
        if name == "bm25_field_weighted":
            continue
        result["paired_vs_bm25_field_weighted"][name] = paired_bootstrap_interval(
            baseline, success
        )
        result["paired_product_cluster_vs_bm25_field_weighted"][name] = (
            paired_product_cluster_bootstrap_interval(intent, baseline, success)
        )

    baseline = success_vectors["bm25_unfielded"]
    for name, success in success_vectors.items():
        if name == "bm25_unfielded":
            continue
        result["paired_vs_bm25_unfielded"][name] = paired_bootstrap_interval(
            baseline, success
        )
        result["paired_product_cluster_vs_bm25_unfielded"][name] = (
            paired_product_cluster_bootstrap_interval(intent, baseline, success)
        )

    if not args.skip_projection_ablation:
        result["projection_ablation"] = evaluate_projection_ablation(
            args.catalog,
            intent,
            forbidden,
            workflows,
            all_queries,
        )

    rendered = json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(rendered, encoding="utf-8")
    else:
        print(rendered, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
