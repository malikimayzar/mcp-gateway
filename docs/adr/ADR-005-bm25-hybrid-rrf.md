# ADR-005: BM25 + Hybrid RRF sebagai Default Retrieval Strategy

- **Status**: Accepted
- **Date**: 2026
- **Author**: Maliki Mayzar

---

## Context

Sistem RAG membutuhkan strategi retrieval yang optimal untuk paper ArXiv. Ada tiga pendekatan utama: dense-only (embedding similarity), sparse-only (BM25 keyword), atau hybrid (kombinasi keduanya).

Keputusan ini didukung oleh ablation study lengkap yang sudah dilakukan di repo `rag-research` dengan zero hallucination sebagai target utama.

---

## Decision

Gunakan **BM25 + Hybrid RRF** (Reciprocal Rank Fusion) sebagai default retrieval strategy, dengan chunk size 256 token.

Tiga mode tersedia via parameter `method`:
- `bm25` — keyword-based, deterministic
- `dense` — semantic similarity via embeddings
- `hybrid` — kombinasi BM25 + dense dengan RRF scoring

Default: `hybrid`

---

## Consequences

### Hasil Ablation Study

| Strategy | Faithfulness | Hallucination Rate | Latency |
|----------|-------------|-------------------|---------|
| Dense only | 0.71 | 12% | ~180ms |
| BM25 only | 0.78 | 6% | ~45ms |
| **Hybrid RRF** | **0.84** | **0%** | ~210ms |

Hybrid RRF menang di faithfulness dan zero hallucination, dengan tradeoff latency yang acceptable.

### Kenapa Hybrid Menang

- **BM25** unggul untuk exact term matching — nama paper, author, teknik spesifik (contoh: "BERT", "attention mechanism")
- **Dense** unggul untuk semantic similarity — query konseptual yang tidak match exact keyword
- **RRF** menggabungkan rank dari kedua metode tanpa perlu tune weight — robust dan tidak overfit ke dataset tertentu

### Chunk Size 256 Token

Ablation study menunjukkan chunk 256 token optimal untuk paper ArXiv:
- Cukup kecil untuk precise retrieval (tidak noise)
- Cukup besar untuk preserve context dalam satu chunk
- Chunk 512 meningkatkan recall tapi menurunkan precision

### Feedback Loop Integration

Ketika faithfulness score < 0.6, sistem otomatis retry dengan strategy berbeda:
```
Attempt 1: hybrid (default)
    │ [score < 0.6]
    ▼
Attempt 2: bm25 (fallback)
    │ [max 2 attempts]
    ▼
Return best result
```

### Tradeoff

- **Latency**: Hybrid ~210ms vs BM25 ~45ms. Untuk research queries, user acceptable dengan latency ini.
- **Complexity**: Maintain dua index (BM25 + FAISS) vs satu. Tradeoff yang worth it mengingat zero hallucination.

### Alternatif yang Ditolak

**Dense-only**: Hallucination rate 12% tidak acceptable untuk research assistant. Dense model bisa "hallucinate" chunk yang semantically similar tapi factually berbeda.

**BM25-only**: Faithfulness lebih rendah (0.78 vs 0.84). Gagal untuk semantic queries yang tidak mengandung exact keyword.

**Cross-encoder reranking**: Signifikan meningkatkan latency (~800ms+) tanpa improvement faithfulness yang cukup besar untuk justify.

---

## Referensi
- Repo `rag-research` — ablation study lengkap dan benchmark results
- `internal/tools/retrieve_chunks.go` — retrieval tool implementation
- `POST /tool` dengan `method: "bm25" | "dense" | "hybrid"`