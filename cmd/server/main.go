package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/malikimayzar/mcp-gateway/internal/registry"
	"github.com/malikimayzar/mcp-gateway/internal/tools"
)

func main() {
	reg := registry.New()
	reg.Register("search_arxiv", tools.SearchArxiv)
	reg.Register("retrieve_chunks", tools.RetrieveChunks)
	reg.Register("evaluate_answer", tools.EvaluateAnswer)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"tools":  reg.List(),
		})
	})

	r.Post("/tool", func(w http.ResponseWriter, r *http.Request) {
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
		json.NewEncoder(w).Encode(resp)
	})

	log.Println("MCP Gateway starting on :8090")
	log.Fatal(http.ListenAndServe(":8090", r))
}
