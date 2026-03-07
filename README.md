# MCP Gateway

> **Self-Hosted AI Research OS** — MCP-based Tool Orchestration with integrated evaluation loop.  
> Polyglot Go+Python · Production-grade · Closed-loop evaluation · Docker Compose ready.

```
┌─────────────────────────────────────────────────────────────┐
│                       MCP Gateway :8090                     │
│                                                             │
│  POST /ask ──► Groq LLM Planner ──► Tool Executor          │
│  POST /plan ──► Rule-based Planner ──► Tool Executor        │
│  POST /tool ──► Direct Tool Call                            │
│                                                             │
│  Tool Registry                                              │
│  ├── search_arxiv    ──► arxiv-research-assistant :8080     │
│  ├── retrieve_chunks ──► rag-research :8003                 │
│  └── evaluate_answer ──► llm-eval-framework :8002           │
│                                                             │
│  Every answer is automatically evaluated for faithfulness   │
│  Low score (<0.6) triggers auto-retry with different method │
└─────────────────────────────────────────────────────────────┘
```

## What This Is

MCP Gateway is an **orchestration layer** that unifies 3 independent AI research services into a single, evaluatable pipeline. Instead of calling each service manually, you send one query and the system decides which tools to use, executes them in order, passes context between steps, and evaluates the final answer — automatically.

This is not a RAG demo. This is a **production-grade AI infrastructure project** built to be deployed, observed, and extended.

---

## Architecture Decision: Why MCP?

| Decision | Choice | Reason |
|---|---|---|
| Protocol | MCP over REST | Tool-oriented contract, schema validation per call, extensible registry |
| Gateway language | Go | Low latency routing, goroutine-based async logging, strong typing |
| ML services | Python | Ecosystem — HuggingFace, FAISS, rank-bm25, sentence-transformers |
| Orchestrator | Groq (llama-3.3-70b) | Fast inference for planning, fallback to rule-based if unavailable |
| Evaluation | Ollama (nomic-embed-text) | Local, no LLM judge needed, faithfulness via cosine similarity |
| Storage | PostgreSQL | Eval scores persisted for trend analysis and Grafana dashboards |

---

## Services

| Service | Stack | Port | Role |
|---|---|---|---|
| **mcp-gateway** | Go + chi | 8090 | Orchestration, tool registry, planner, executor |
| **arxiv-research-assistant** | Go + Python + Qdrant | 8080/8001 | ArXiv search, paper ingestion, RAG |
| **rag-research** | Python + FAISS + BM25 | 8003 | Hybrid retrieval (BM25 + dense RRF) |
| **llm-eval-framework** | Python + Ollama | 8002 | Faithfulness evaluation, failure mode analysis |
| **PostgreSQL** | postgres:16 | 5432 | Query logs, eval scores, paper metadata |
| **Qdrant** | qdrant/qdrant | 6333 | Vector store for dense retrieval |
| **Grafana** | grafana/grafana | 3000 | Observability dashboard |
| **Prometheus** | prom/prometheus | 9090 | Metrics collection |

---

## Quick Start

**Requirements:** Docker, Docker Compose, Ollama (with `nomic-embed-text` model)

```bash
# Clone all repos into the same directory
git clone https://github.com/malikimayzar/mcp-gateway
git clone https://github.com/malikimayzar/arxiv-research-assistant
git clone https://github.com/malikimayzar/llm-eval-framework
git clone https://github.com/malikimayzar/rag-research

# Set environment
cp mcp-gateway/.env.example mcp-gateway/.env
# Edit .env and add your GROQ_API_KEY

# Start everything
docker compose up -d

# Wait ~30s for services to load models, then test
curl -s -X POST http://localhost:8090/ask \
  -H "Content-Type: application/json" \
  -d '{"query": "what is RAG?", "top_k": 3}' | jq .
```

---

## API

### `POST /ask` — LLM-Orchestrated Pipeline
Groq LLM decides which tools to call and in what order. Falls back to rule-based if Groq is unavailable.

```bash
curl -X POST http://localhost:8090/ask \
  -H "Content-Type: application/json" \
  -d '{"query": "what is attention mechanism?", "top_k": 5}'
```

```json
{
  "Query": "what is attention mechanism?",
  "Answer": "...",
  "Score": 0.8611,
  "orchestrator": "groq",
  "Steps": [
    {"ToolName": "retrieve_chunks", "Success": true},
    {"ToolName": "evaluate_answer", "Success": true}
  ]
}
```

### `POST /plan` — Rule-based Pipeline + Auto-retry
Keyword-based tool selection. Auto-retries with different retrieval strategy if faithfulness score < 0.6.

```bash
curl -X POST http://localhost:8090/plan \
  -H "Content-Type: application/json" \
  -d '{"query": "what is BM25?", "top_k": 5}'
```

### `POST /tool` — Direct Tool Call
Call any registered tool directly.

```bash
curl -X POST http://localhost:8090/tool \
  -H "Content-Type: application/json" \
  -d '{
    "tool_name": "retrieve_chunks",
    "params": {"query": "transformer architecture", "top_k": 5, "method": "hybrid"}
  }'
```

### `GET /health`
```json
{"status": "ok", "tools": ["search_arxiv", "retrieve_chunks", "evaluate_answer"]}
```

---

## Benchmark Results

Measured on local machine (WSL2, CPU only):

| Tool / Endpoint | Avg Latency | Min | Max | Faithfulness |
|---|---|---|---|---|
| `retrieve_chunks` | 113ms | 34ms | 525ms | — |
| `evaluate_answer` | 3.3s | 2.5s | 4.6s | 1.00 |
| `POST /plan` | 57s | 54s | 60s | 0.75 |
| `POST /ask` (Groq) | 76s | 75s | 77s | 0.86 |

> Note: `/plan` and `/ask` latency dominated by `evaluate_answer` (Ollama embedding on CPU).  
> `retrieve_chunks` alone: ~34-113ms for hybrid BM25+FAISS retrieval.

---

## Evaluation Loop

Every answer produced by the pipeline is **automatically evaluated for faithfulness** — no human in the loop, no LLM judge.

```
Query → retrieve_chunks → [answer + context] → evaluate_answer
                                                      │
                                              faithfulness_score
                                                      │
                                            score < 0.6? ──► retry with bm25
                                                      │
                                              store to PostgreSQL
```

Evaluation results are persisted to `query_logs` table:
```sql
SELECT query, faithfulness, model, retrieval_strategy, created_at
FROM query_logs ORDER BY created_at DESC LIMIT 10;
```

---

## Project Structure

```
mcp-gateway/
├── cmd/server/main.go          — entry point, chi router, /ask /plan /tool
├── internal/
│   ├── registry/               — tool registration & routing
│   ├── tools/
│   │   ├── search_arxiv.go     — calls arxiv-research-assistant
│   │   ├── retrieve_chunks.go  — calls rag-research
│   │   └── evaluate_answer.go  — calls llm-eval-framework
│   ├── planner/
│   │   ├── planner.go          — rule-based tool selector
│   │   └── executor.go         — sequential execution + retry logic
│   ├── orchestrator/
│   │   └── groq.go             — Groq LLM orchestrator + JSON extraction
│   └── store/
│       └── postgres.go         — eval score persistence
├── scripts/
│   └── benchmark.py            — latency + faithfulness benchmark
├── docs/tool-contracts/        — JSON specs per tool
├── docker-compose.yml          — full stack (10 services)
└── Makefile                    — start/stop/status/logs
```

---

## Key Engineering Decisions

**1. Closed-loop evaluation by default**  
Every answer is evaluated. Not optional, not a flag. The system treats unevaluated answers as incomplete.

**2. Groq as planner, not generator**  
Groq is only used for *deciding which tools to call* — not for generating the answer. This keeps the system cost-efficient and the answers grounded in retrieved context.

**3. Fallback chain**  
`Groq plan → rule-based plan → individual tool`. The system degrades gracefully at every layer.

**4. Polyglot by necessity, not trend**  
Go handles routing, concurrency, and low-latency HTTP. Python handles ML — embeddings, retrieval, evaluation. Each language does what it's best at.

---

## Environment Variables

```bash
# Required
GROQ_API_KEY=gsk_...          # Groq API key for LLM orchestration

# Optional (defaults shown)
POSTGRES_DSN=host=postgres port=5432 user=arxiv password=arxiv_secret dbname=arxiv_db sslmode=disable
RAG_SERVICE_URL=http://arxiv-rag-service:8003
EVAL_SERVICE_URL=http://arxiv-eval-service:8002
ARXIV_SERVICE_URL=http://arxiv-go-backend:8080
```

---

## Related Repos

| Repo | Description |
|---|---|
| [arxiv-research-assistant](https://github.com/malikimayzar/arxiv-research-assistant) | Go+Python RAG system, ArXiv ingestion, circuit breaker, observability |
| [llm-eval-framework](https://github.com/malikimayzar/llm-eval-framework) | Faithfulness evaluation without LLM judge, 105 unit tests |
| [rag-research](https://github.com/malikimayzar/rag-research) | BM25+FAISS hybrid retrieval, ablation study, zero hallucination benchmark |

---

*Maliki Mayzar · 2026*