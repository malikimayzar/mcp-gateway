# MCP Gateway

A Model Context Protocol (MCP) gateway that orchestrates multiple AI research services into a unified, evaluatable pipeline.

## Architecture
```
MCP Gateway (:8090)
├── /tool          — direct tool call
├── /plan          — auto-orchestrated pipeline with eval
└── /health        — registered tools list

Tools:
├── search_arxiv    → arxiv-research-assistant (:8080)
├── retrieve_chunks → rag-research (:8003)
└── evaluate_answer → llm-eval-framework (:8002)
```

## Features

- **Tool Registry** — register and route tool calls by name
- **Planner** — rule-based query analysis to select optimal tools and retrieval method
- **Executor** — sequential tool execution with context passing between steps
- **Eval Loop** — automatic faithfulness scoring with retry on low score (<0.6)

## Quick Start
```bash
# Start all dependencies first
cd arxiv-research-assistant/go-backend && go run cmd/server/main.go   # :8080
cd arxiv-research-assistant/python-ml && uvicorn main:app --port 8001
cd llm-eval-framework && uvicorn api:app --port 8002
cd rag-research && uvicorn api:app --port 8003

# Start MCP Gateway
cd mcp-gateway
go run cmd/server/main.go  # :8090
```

## API

### POST /tool — Direct tool call
```json
{
  "tool_name": "retrieve_chunks",
  "params": {"query": "What is RAG?", "top_k": 5, "method": "hybrid"},
  "trace_id": "optional"
}
```

### POST /plan — Auto-orchestrated pipeline
```json
{
  "query": "What are the three stages of RAG?",
  "top_k": 5
}
```

Response includes:
- Retrieved chunks
- Faithfulness score (0.0 - 1.0)
- Auto-retry if score < 0.6

### GET /health
```json
{"status": "ok", "tools": ["search_arxiv", "retrieve_chunks", "evaluate_answer"]}
```

## Tool Contracts

See `docs/tool-contracts/` for full input/output specs per tool.

## Stack

- **Gateway**: Go + chi router
- **RAG Service**: Go + Python (Fiber, FastAPI, Qdrant, PostgreSQL)
- **Retrieval**: Python (BM25 + FAISS hybrid, sentence-transformers)
- **Evaluation**: Python (faithfulness scoring, nomic-embed-text)
