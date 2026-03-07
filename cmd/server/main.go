package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/malikimayzar/mcp-gateway/internal/orchestrator"
	"github.com/malikimayzar/mcp-gateway/internal/planner"
	"github.com/malikimayzar/mcp-gateway/internal/registry"
	"github.com/malikimayzar/mcp-gateway/internal/store"
	"github.com/malikimayzar/mcp-gateway/internal/tools"
)

func main() {
	store.Init()
	defer store.Close()

	reg := registry.New()
	reg.Register("search_arxiv", tools.SearchArxiv)
	reg.Register("retrieve_chunks", tools.RetrieveChunks)
	reg.Register("evaluate_answer", tools.EvaluateAnswer)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	// GET /health — list registered tools
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"tools":  reg.List(),
		})
	})

	// POST /tool — direct tool call
	r.Post("/tool", func(w http.ResponseWriter, _ *http.Request) {
		var req registry.ToolRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.TraceID == "" {
			req.TraceID = middleware.GetReqID(r.Context())
		}

		ctx, cancel := context.WithTimeout(r.Context(), 620*time.Second)
		defer cancel()

		resp := reg.Execute(ctx, req)
		w.Header().Set("Content-Type", "application/json")
		if !resp.Success {
			w.WriteHeader(http.StatusBadRequest)
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	// POST /plan — rule-based planner + eval loop
	r.Post("/plan", func(w http.ResponseWriter, _ *http.Request) {
		var body struct {
			Query string `json:"query"`
			TopK  int    `json:"top_k"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
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

		ctx, cancel := context.WithTimeout(r.Context(), 620*time.Second)
		defer cancel()

		result := planner.ExecuteWithRetry(ctx, reg, body.Query, body.TopK)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})

	// POST /ask — Groq LLM orchestrator, fallback ke rule-based
	r.Post("/ask", func(w http.ResponseWriter, _ *http.Request) {
		var body struct {
			Query string `json:"query"`
			TopK  int    `json:"top_k"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
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

		ctx, cancel := context.WithTimeout(r.Context(), 620*time.Second)
		defer cancel()

		// Coba Groq dulu
		orcPlan, err := orchestrator.Plan(ctx, body.Query)
		if err != nil {
			// Fallback ke rule-based planner
			log.Printf("[ask] Groq failed (%v), falling back to rule-based planner", err)
			result := planner.ExecuteWithRetry(ctx, reg, body.Query, body.TopK)

			// Tambah field orchestrator = "rule-based"
			go store.LogEval(ctx, store.EvalLog{
				Query:             body.Query,
				Answer:            result.Answer,
				Faithfulness:      result.Score,
				FailureMode:       store.ExtractFailureMode(evalData(result)),
				RetrievalStrategy: "rule-based",
				Orchestrator:      "rule-based",
				Retried:           result.Retried,
			})
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(withOrchestrator(result, "rule-based"))
			return
		}

		log.Printf("[ask] using Groq plan (%d steps)", len(orcPlan.Steps))

		// Build execPlan dari Groq plan
		execPlan := planner.Plan{Query: body.Query}
		for _, step := range orcPlan.Steps {
			if step.Params == nil {
				step.Params = map[string]interface{}{}
			}
			if _, ok := step.Params["query"]; !ok {
				step.Params["query"] = body.Query
			}
			if _, ok := step.Params["top_k"]; !ok {
				step.Params["top_k"] = float64(body.TopK)
			}
			execPlan.Steps = append(execPlan.Steps, planner.Step{
				ToolName: step.ToolName,
				Params:   step.Params,
			})
		}

		result := planner.Execute(ctx, reg, execPlan)

		// Tambah field orchestrator = "groq"
		go store.LogEval(ctx, store.EvalLog{
			Query:             body.Query,
			Answer:            result.Answer,
			Faithfulness:      result.Score,
			FailureMode:       store.ExtractFailureMode(evalData(result)),
			RetrievalStrategy: "hybrid",
			Orchestrator:      "groq",
			Retried:           result.Retried,
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(withOrchestrator(result, "groq"))
	})

	log.Println("MCP Gateway starting on :8090")
	log.Fatal(http.ListenAndServe(":8090", r))
}

// withOrchestrator menyuntikkan field "orchestrator" ke result map
// tanpa mengubah struct planner.Result yang sudah ada.
func withOrchestrator(result interface{}, name string) map[string]interface{} {
	// Marshal result ke JSON dulu
	b, err := json.Marshal(result)
	if err != nil {
		return map[string]interface{}{"orchestrator": name, "error": "marshal failed"}
	}

	// Unmarshal ke map supaya bisa tambah field
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]interface{}{"orchestrator": name, "error": "unmarshal failed"}
	}

	m["orchestrator"] = name
	return m
}

// evalData ambil Data dari step evaluate_answer kalau ada
func evalData(result planner.ExecutionResult) map[string]interface{} {
	for _, step := range result.Steps {
		if step.ToolName == "evaluate_answer" && step.Success {
			return step.Data
		}
	}
	return nil
}
