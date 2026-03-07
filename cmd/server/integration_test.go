package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/malikimayzar/mcp-gateway/internal/planner"
	"github.com/malikimayzar/mcp-gateway/internal/registry"
)

func newTestRegistry() *registry.Registry {
	reg := registry.New()

	reg.Register("search_arxiv", func(_ context.Context, req registry.ToolRequest) registry.ToolResponse {
		return registry.ToolResponse{
			ToolName: req.ToolName,
			TraceID:  req.TraceID,
			Success:  true,
			Data: map[string]interface{}{
				"papers": []map[string]string{
					{"title": "Stub Paper A", "abstract": "stub abstract about " + req.Params["query"].(string)},
				},
			},
		}
	})

	reg.Register("retrieve_chunks", func(_ context.Context, req registry.ToolRequest) registry.ToolResponse {
		return registry.ToolResponse{
			ToolName: req.ToolName,
			TraceID:  req.TraceID,
			Success:  true,
			Data: map[string]interface{}{
				"chunks": []string{"chunk A", "chunk B"},
			},
		}
	})

	reg.Register("evaluate_answer", func(_ context.Context, req registry.ToolRequest) registry.ToolResponse {
		return registry.ToolResponse{
			ToolName: req.ToolName,
			TraceID:  req.TraceID,
			Success:  true,
			Data: map[string]interface{}{
				"faithfulness": 0.85,
				"consistency":  0.90,
				"failure_mode": "none",
			},
		}
	})

	return reg
}

func newTestServer(reg *registry.Registry) http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"tools":  reg.List(),
		})
	})

	r.Post("/tool", func(w http.ResponseWriter, req *http.Request) {
		var toolReq registry.ToolRequest
		if err := json.NewDecoder(req.Body).Decode(&toolReq); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if toolReq.TraceID == "" {
			toolReq.TraceID = chimiddleware.GetReqID(req.Context())
		}
		ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
		defer cancel()

		resp := reg.Execute(ctx, toolReq)
		w.Header().Set("Content-Type", "application/json")
		if !resp.Success {
			w.WriteHeader(http.StatusBadRequest)
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	r.Post("/plan", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Query string `json:"query"`
			TopK  int    `json:"top_k"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if body.Query == "" {
			http.Error(w, "query is required", http.StatusBadRequest)
			return
		}
		if body.TopK == 0 {
			body.TopK = 5
		}
		ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
		defer cancel()

		result := planner.ExecuteWithRetry(ctx, reg, body.Query, body.TopK)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})

	return r
}

func postJSON(t *testing.T, ts *httptest.Server, path string, body interface{}) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request %s: %v", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	var m map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return m
}

func keys(m map[string]interface{}) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

func TestHealth(t *testing.T) {
	ts := httptest.NewServer(newTestServer(newTestRegistry()))
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/health", nil)
	if err != nil {
		t.Fatalf("new request /health: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	body := decodeJSON(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", body["status"])
	}
	tools, ok := body["tools"].([]interface{})
	if !ok || len(tools) == 0 {
		t.Errorf("expected non-empty tools list, got %v", body["tools"])
	}
}

func TestTool_SearchArxiv(t *testing.T) {
	ts := httptest.NewServer(newTestServer(newTestRegistry()))
	defer ts.Close()

	resp := postJSON(t, ts, "/tool", map[string]interface{}{
		"ToolName": "search_arxiv",
		"TraceID":  "trace-integ-001",
		"Params":   map[string]interface{}{"query": "hallucination in RAG"},
	})
	body := decodeJSON(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if body["Success"] != true {
		t.Errorf("expected Success=true, got %v", body["Success"])
	}
	if body["TraceID"] != "trace-integ-001" {
		t.Errorf("expected TraceID propagated, got %v", body["TraceID"])
	}
}

func TestTool_RetrieveChunks(t *testing.T) {
	ts := httptest.NewServer(newTestServer(newTestRegistry()))
	defer ts.Close()

	resp := postJSON(t, ts, "/tool", map[string]interface{}{
		"ToolName": "retrieve_chunks",
		"Params":   map[string]interface{}{"query": "attention mechanism", "top_k": 3},
	})
	body := decodeJSON(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if body["Success"] != true {
		t.Errorf("expected Success=true, got %v", body["Success"])
	}
}

func TestTool_EvaluateAnswer(t *testing.T) {
	ts := httptest.NewServer(newTestServer(newTestRegistry()))
	defer ts.Close()

	resp := postJSON(t, ts, "/tool", map[string]interface{}{
		"ToolName": "evaluate_answer",
		"Params": map[string]interface{}{
			"answer":   "RAG reduces hallucination by grounding answers in retrieved context.",
			"context":  "chunk A chunk B",
			"question": "How does RAG reduce hallucination?",
		},
	})
	body := decodeJSON(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if body["Success"] != true {
		t.Errorf("expected Success=true, got %v", body["Success"])
	}
}

func TestTool_NotFound(t *testing.T) {
	ts := httptest.NewServer(newTestServer(newTestRegistry()))
	defer ts.Close()

	resp := postJSON(t, ts, "/tool", map[string]interface{}{
		"ToolName": "nonexistent_tool",
	})
	body := decodeJSON(t, resp)

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	if body["Success"] == true {
		t.Error("expected Success=false for unknown tool")
	}
	if body["Error"] == "" {
		t.Error("expected non-empty Error message")
	}
}

func TestTool_InvalidBody(t *testing.T) {
	ts := httptest.NewServer(newTestServer(newTestRegistry()))
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/tool", bytes.NewBufferString("not-json"))
	if err != nil {
		t.Fatalf("new request /tool: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /tool: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestPlan_ReturnsResult(t *testing.T) {
	ts := httptest.NewServer(newTestServer(newTestRegistry()))
	defer ts.Close()

	resp := postJSON(t, ts, "/plan", map[string]interface{}{
		"query": "What are recent papers on RAG hallucination?",
		"top_k": 3,
	})
	body := decodeJSON(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if _, ok := body["Answer"]; !ok {
		t.Errorf("expected Answer field in response, got keys: %v", keys(body))
	}
}

func TestPlan_EmptyQuery(t *testing.T) {
	ts := httptest.NewServer(newTestServer(newTestRegistry()))
	defer ts.Close()

	resp := postJSON(t, ts, "/plan", map[string]interface{}{
		"query": "",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for empty query, got %d", resp.StatusCode)
	}
}

func TestPlan_DefaultTopK(t *testing.T) {
	ts := httptest.NewServer(newTestServer(newTestRegistry()))
	defer ts.Close()

	resp := postJSON(t, ts, "/plan", map[string]interface{}{
		"query": "transformer architecture overview",
	})
	body := decodeJSON(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if _, ok := body["Answer"]; !ok {
		t.Errorf("expected Answer field, got: %v", keys(body))
	}
}

func TestPlan_InvalidBody(t *testing.T) {
	ts := httptest.NewServer(newTestServer(newTestRegistry()))
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/plan", bytes.NewBufferString("{bad json}"))
	if err != nil {
		t.Fatalf("new request /plan: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /plan: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
}
