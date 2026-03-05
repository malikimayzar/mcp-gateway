package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/malikimayzar/mcp-gateway/internal/registry"
)

const retrieveChunksURL = "http://localhost:8003/retrieve"

func RetrieveChunks(ctx context.Context, req registry.ToolRequest) registry.ToolResponse {
	query, ok := req.Params["query"].(string)
	if !ok || query == "" {
		return errorResponse(req, "param query is required")
	}

	topK := 5
	if v, ok := req.Params["top_k"].(float64); ok {
		topK = int(v)
	}

	method := "bm25"
	if v, ok := req.Params["method"].(string); ok && v != "" {
		method = v
	}

	body, _ := json.Marshal(map[string]interface{}{
		"query":  query,
		"top_k":  topK,
		"method": method,
	})

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, "POST", retrieveChunksURL, bytes.NewBuffer(body))
	if err != nil {
		return errorResponse(req, fmt.Sprintf("failed to build request: %v", err))
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return errorResponse(req, fmt.Sprintf("retrieve_chunks call failed: %v", err))
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return errorResponse(req, fmt.Sprintf("retrieve_chunks returned %d: %s", resp.StatusCode, string(respBody)))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return errorResponse(req, fmt.Sprintf("failed to parse response: %v", err))
	}

	return registry.ToolResponse{
		ToolName: req.ToolName,
		TraceID:  req.TraceID,
		Success:  true,
		Data:     result,
	}
}
