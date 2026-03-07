# ADR-001: MCP sebagai Protocol Gateway, bukan REST biasa

- **Status**: Accepted
- **Date**: 2026
- **Author**: Maliki Mayzar

---

## Context

MCP Gateway perlu mengekspos multiple AI tools (search, retrieval, evaluation) ke client dalam satu unified interface. Ada dua pendekatan yang dipertimbangkan: REST API biasa atau Model Context Protocol (MCP).

Dengan REST biasa, setiap tool jadi endpoint terpisah (`POST /search`, `POST /retrieve`, `POST /evaluate`). Client harus tahu persis endpoint mana yang dipanggil, format request-nya, dan urutan pemanggilan.

MCP adalah protocol yang dirancang khusus untuk AI tool orchestration — client cukup tahu nama tool dan schema-nya, bukan detail implementasi.

---

## Decision

Gunakan **MCP-inspired protocol** dengan tool registry terpusat di Go, bukan REST endpoint per tool.

Semua tool call masuk lewat satu entry point (`POST /tool`) dengan payload:
```json
{
  "tool_name": "search_arxiv",
  "params": { "query": "...", "top_k": 5 },
  "trace_id": "req-abc123"
}
```

---

## Consequences

### Keuntungan
- **Single interface** — client tidak perlu tahu ada berapa tool atau di mana mereka jalan
- **Schema validation terpusat** — setiap tool punya JSON schema, validasi terjadi di gateway sebelum diteruskan ke service
- **Composability** — Planner bisa chain tool calls tanpa client involvement
- **Observability** — semua tool call lewat satu titik, trace ID konsisten di seluruh sistem
- **Extensibility** — tambah tool baru = register handler baru, tidak ubah API contract

### Tradeoff
- Lebih kompleks dari REST sederhana untuk use case single-tool
- Client tidak bisa langsung hit individual service (by design — ini fitur, bukan bug)
- Debugging butuh trace ID untuk follow request path

### Alternatif yang Ditolak
**REST per tool**: Lebih simple awalnya, tapi tidak scale ketika jumlah tool bertambah. Tidak ada cara natural untuk chaining dan evaluation loop.

**GraphQL**: Overkill untuk use case ini. Schema complexity tidak sebanding dengan benefit.

---

## Referensi
- [Model Context Protocol spec](https://modelcontextprotocol.io)
- `internal/registry/registry.go` — implementasi tool registry
- `docs/tool-contracts/` — JSON schema per tool