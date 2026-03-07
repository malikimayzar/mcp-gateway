# ADR-004: Qdrant sebagai Vector Store

- **Status**: Accepted
- **Date**: 2026
- **Author**: Maliki Mayzar

---

## Context

Sistem RAG membutuhkan vector store untuk menyimpan dan melakukan similarity search terhadap embeddings paper ArXiv. Kandidat utama: Qdrant, Pinecone, Weaviate, ChromaDB, atau FAISS in-memory.

Kriteria evaluasi: self-hostable, performa similarity search, kemudahan operasional, dan Docker support.

---

## Decision

Gunakan **Qdrant** sebagai vector store utama untuk arxiv-research-assistant.

---

## Consequences

### Kenapa Qdrant

- **Self-hosted** — berjalan sebagai Docker container, tidak ada cloud dependency atau biaya per-query
- **Performa** — HNSW index dengan filtering, sub-millisecond search untuk collection skala jutaan vector
- **REST + gRPC API** — mudah diintegrasikan dari Python maupun Go
- **Persistent storage** — data survive restart, tidak perlu re-index ulang setiap startup
- **Payload filtering** — bisa filter berdasarkan metadata (year, category, author) bersamaan dengan vector search
- **Production-ready** — collection management, snapshots, dan monitoring built-in

### Tradeoff

- **Operational overhead** — satu service tambahan di Docker Compose vs embedded solution
- **Memory usage** — HNSW index perlu RAM yang cukup untuk collection besar
- **Mitigasi**: Untuk skala paper ArXiv yang digunakan (ribuan, bukan jutaan), resource usage sangat manageable

### Alternatif yang Ditolak

**FAISS in-memory**: Digunakan di `rag-research` untuk BM25+Hybrid retrieval karena lebih ringan untuk prototype. Tidak persistent — setiap restart butuh re-index. Tidak cocok untuk production dengan data besar.

**Pinecone**: Managed service, tidak self-hostable. Biaya per-query tidak predictable. Tidak sesuai dengan prinsip self-hosted sistem ini.

**ChromaDB**: Lebih mudah setup, tapi performa dan fitur filtering kalah dari Qdrant untuk use case production.

**Weaviate**: Feature-rich tapi lebih complex dari yang dibutuhkan. Resource usage lebih besar.

---

## Referensi
- `docker-compose.yml` — Qdrant service configuration
- Repo `arxiv-research-assistant` — Qdrant client integration
- [Qdrant documentation](https://qdrant.tech/documentation/)