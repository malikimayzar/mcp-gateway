# MCP Gateway — Postmortem: 5 Failure Scenarios

**Project:** MCP Gateway — AI Research Toolchain  
**Author:** Maliki Mayzar  
**Stack:** Go + Python · Groq API · PostgreSQL · Docker Compose  
**Repo:** github.com/malikimayzar/mcp-gateway  

---

## Overview

Dokumen ini mendokumentasikan 5 failure scenario yang diidentifikasi selama development dan testing MCP Gateway. Setiap scenario mencakup root cause, impact, detection method, dan mitigasi yang diimplementasikan atau direncanakan.

---

## Failure Scenario 1: Unsupported Claims (Hallucination)

**Failure Mode:** `unsupported_claims`  
**Severity:** High  
**Status:** Detected & Mitigated

### What Happened

Sistem menghasilkan jawaban yang mengandung klaim yang tidak bisa diverifikasi dari chunks yang di-retrieve. LLM menambahkan informasi di luar konteks dokumen yang tersedia — khususnya pada query tentang topik yang tidak cukup terwakili di corpus arxiv.

**Contoh query yang memicu:**
```
"What are the latest results from GPT-5 on reasoning benchmarks?"
```

Groq mengembalikan jawaban dengan angka benchmark spesifik yang tidak ada di retrieved chunks. `evaluate_answer` mendeteksi ini dan memberi label `unsupported_claims`.

### Root Cause

Groq LLM (llama3-8b-8192) cenderung mengisi gap konteks dengan pengetahuan parametric-nya ketika retrieved chunks tidak cukup menjawab query. Ini adalah perilaku default yang tidak dibatasi di prompt orchestrator.

### Impact

- Faithfulness score turun ke range 0.5–0.7
- Answer tidak bisa dipercaya untuk research use case
- User mendapat informasi yang terlihat valid tapi tidak grounded

### Detection

`evaluate_answer` tool mendeteksi via faithfulness scoring. Query dengan failure mode ini ter-log di `query_logs` dengan kolom `failure_mode = 'unsupported_claims'`.

### Mitigasi yang Diimplementasikan

1. Auto-retry dengan retrieval strategy berbeda kalau faithfulness < 0.6
2. Grafana alert aktif kalau avg faithfulness drop di bawah threshold
3. Panel "Recent Low-Confidence Queries" di dashboard untuk monitoring

### Lesson Learned

System prompt Groq perlu instruksi eksplisit: *"Only answer based on the provided context. If context is insufficient, say so."* Ini akan diimplementasikan di iterasi berikutnya.

---

## Failure Scenario 2: Insufficient Context (Retrieval Miss)

**Failure Mode:** `insufficient_context`  
**Severity:** Medium  
**Status:** Partially Mitigated

### What Happened

Query yang terlalu spesifik atau menggunakan terminologi yang berbeda dari corpus menyebabkan retrieval mengembalikan chunks yang tidak relevan. `retrieve_chunks` berhasil mengembalikan hasil, tapi kontennya tidak menjawab pertanyaan.

**Contoh query yang memicu:**
```
"Explain the Mamba SSM architecture's advantage over standard attention"
```

BM25 retrieval mengembalikan paper tentang attention mechanism secara umum, bukan Mamba-specific content. Answer yang dihasilkan generik dan tidak membantu.

### Root Cause

BM25 bergantung pada exact keyword match. Query yang menggunakan sinonim atau parafrase tidak ter-cover dengan baik. Dense retrieval lebih robust tapi belum jadi default strategy.

### Impact

- Answer generik dan tidak informatif
- Faithfulness score bisa tinggi (karena answer grounded ke chunks yang ada) tapi answer tidak berguna
- Consistency score rendah karena answer tidak konsisten dengan intent query

### Detection

Kombinasi faithfulness tinggi tapi consistency rendah adalah sinyal kuat untuk insufficient context. Saat ini detection dilakukan via manual review panel "Failure Mode Distribution" di Grafana.

### Mitigasi yang Diimplementasikan

1. Hybrid RRF (BM25 + dense) sebagai strategi default di `/ask` endpoint
2. Retry mechanism: kalau score rendah, coba strategy berbeda
3. `top_k` default = 5 untuk memperluas recall

### Lesson Learned

Ablation study dari `rag-research` repo sudah membuktikan hybrid RRF unggul. Ke depan, query classification perlu diimplementasikan — query tentang specific model architecture langsung pakai dense retrieval.

---

## Failure Scenario 3: Groq API Timeout / Rate Limit

**Failure Mode:** Orchestrator Failure → Fallback ke Rule-based  
**Severity:** Medium  
**Status:** Mitigated via Fallback

### What Happened

Groq API mengembalikan error atau timeout pada periode high load. Terjadi dua kondisi berbeda:

1. **Rate limit (429):** Terlalu banyak request dalam window tertentu
2. **Timeout:** Response time Groq melebihi batas context deadline

Sistem crash tanpa fallback pada implementasi awal — request ke `/ask` mengembalikan 500 error.

### Root Cause

Tidak ada fallback mechanism di versi awal orchestrator. `context.WithTimeout` dipasang tapi kalau Groq gagal, tidak ada recovery path.

### Impact

- `/ask` endpoint mengembalikan 500
- Semua query yang masuk selama downtime tidak ter-serve
- Tidak ada visibility ke berapa banyak request yang gagal

### Detection

Grafana panel "Groq vs Rule-based" menunjukkan persentase rule-based naik drastis saat Groq bermasalah. Log `[ask] Groq failed, falling back to rule-based planner` muncul di mcp-gateway logs.

### Mitigasi yang Diimplementasikan

```go
// main.go — fallback logic
orcPlan, err := orchestrator.GeneratePlan(ctx, body.Query)
if err != nil {
    log.Printf("[ask] Groq failed (%v), falling back to rule-based planner", err)
    result := planner.ExecuteWithRetry(ctx, reg, body.Query, body.TopK)
    // ... return result dengan orchestrator="rule-based"
}
```

Fallback ke rule-based planner memastikan sistem tetap melayani request meski Groq down.

### Lesson Learned

External API dependency harus selalu punya fallback. Circuit breaker pattern (sudah ada di `arxiv-research-assistant`) perlu di-port ke MCP Gateway untuk mencegah cascade failure.

---

## Failure Scenario 4: PostgreSQL Connection Failure (Store Unavailable)

**Failure Mode:** Store Write Failure  
**Severity:** Low (untuk query serving) / Medium (untuk observability)  
**Status:** Mitigated via Async Logging

### What Happened

PostgreSQL container restart atau connection pool exhausted menyebabkan `store.LogEval()` gagal. Pada implementasi awal, ini menyebabkan request handler ikut gagal karena store write dilakukan secara synchronous.

**Contoh error:**
```
failed to connect to server - please inspect Grafana server log for details
```

Error ini juga terlihat di Grafana alert evaluation karena alert engine tidak bisa query `query_logs`.

### Root Cause

Store write dilakukan inline di request handler. Kalau postgres down, write gagal dan bisa propagate ke response.

### Impact

- Query tetap bisa di-serve (answer tetap dikembalikan)
- Tapi eval data tidak ter-log → Grafana dashboard kosong
- Alert evaluation gagal karena tidak bisa connect ke DB

### Detection

Grafana logs: `db query error: failed to connect to server`. Panel "Total Queries" di dashboard menunjukkan angka stagnan meski traffic masuk.

### Mitigasi yang Diimplementasikan

```go
// main.go — async logging dengan goroutine
go store.LogEval(ctx, store.EvalLog{
    Query:        body.Query,
    Answer:       result.Answer,
    Faithfulness: result.Score,
    // ...
})
```

Store write dijadikan async via goroutine — request handler tidak blocked oleh DB write. Answer tetap dikembalikan ke user meski logging gagal.

### Lesson Learned

Observability path (logging) tidak boleh blocking untuk serving path (response). Ke depan, buffer queue (channel) perlu ditambahkan agar log tidak hilang saat DB temporary unavailable.

---

## Failure Scenario 5: Tool Registry — Unknown Tool Call

**Failure Mode:** Tool Not Found  
**Severity:** Low  
**Status:** Handled

### What Happened

Groq orchestrator kadang menghasilkan execution plan dengan tool name yang tidak terdaftar di registry. Ini terjadi ketika Groq "hallucinate" tool name yang tidak persis sama dengan yang didefinisikan.

**Contoh:**
```json
// Groq generate plan dengan tool name yang salah
{"tool_name": "search_papers"}  // seharusnya "search_arxiv"
{"tool_name": "retrieve"}       // seharusnya "retrieve_chunks"
```

### Root Cause

Tool contract tidak di-inject ke Groq system prompt secara eksplisit. Groq menebak nama tool berdasarkan konteks query, bukan dari definisi yang diberikan.

### Impact

- Step dalam execution plan di-skip dengan error "tool not found"
- Answer dihasilkan dari partial execution — bisa incomplete atau misleading
- Tidak ada visibility ke berapa banyak plan steps yang di-skip

### Detection

`registry.Execute()` mengembalikan `ToolResponse{Success: false, Error: "tool not found"}`. Error ini ter-log di execution result steps tapi tidak selalu surfaced ke user response.

Unit test yang sudah ada mencover skenario ini:
```go
// registry_test.go
func TestExecute_ToolNotFound(t *testing.T) {
    resp := r.Execute(context.Background(), ToolRequest{
        ToolName: "nonexistent",
    })
    if resp.Success { t.Error("expected failure") }
}
```

### Mitigasi yang Diimplementasikan

1. `registry.Execute()` return error yang jelas dengan tool name di message
2. Executor mencatat failed steps di `ExecutionResult.Steps`
3. `/tool` endpoint return HTTP 400 untuk unknown tool

### Planned Improvement

Inject tool contracts (dari `docs/tool-contracts/*.json`) ke Groq system prompt agar LLM tahu exact tool names yang tersedia. Ini akan mengeliminasi hallucinated tool names.

---

## Summary

| # | Failure Mode | Severity | Detection | Status |
|---|-------------|----------|-----------|--------|
| 1 | Unsupported Claims (Hallucination) | High | Faithfulness score < 0.6 | Mitigated |
| 2 | Insufficient Context (Retrieval Miss) | Medium | Low consistency score | Partially Mitigated |
| 3 | Groq API Timeout / Rate Limit | Medium | Grafana: Groq% drop | Mitigated |
| 4 | PostgreSQL Connection Failure | Low/Medium | Grafana: query count stagnan | Mitigated |
| 5 | Unknown Tool Call (Registry Miss) | Low | ToolResponse.Success=false | Handled |

---

## What This Shows

Sistem ini bukan sekedar RAG pipeline — ada **closed-loop evaluation** yang aktif mendeteksi failure, **fallback mechanism** untuk external API dependency, dan **async observability** yang tidak mengorbankan serving path. Setiap failure scenario di atas sudah punya detection path yang visible di Grafana dashboard.

---

*Maliki Mayzar · MCP Gateway · 2026*