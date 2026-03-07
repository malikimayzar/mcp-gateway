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

const evaluateAnswerURL = "http://arxiv-eval-service:8002/evaluate"

func EvaluateAnswer(ctx context.Context, req registry.ToolRequest) registry.ToolResponse {
	answer, ok := req.Params["answer"].(string)
	if !ok || answer == "" {
		return errorResponse(req, "param answer is required")
	}

	contextStr, ok := req.Params["context"].(string)
	if !ok || contextStr == "" {
		return errorResponse(req, "param context is required")
	}

	question, ok := req.Params["question"].(string)
	if !ok || question == "" {
		return errorResponse(req, "param question is required")
	}

	body, _ := json.Marshal(map[string]interface{}{
		"answer":   answer,
		"context":  contextStr,
		"question": question,
	})

	// Pakai context baru supaya tidak terpengaruh deadline parent
	evalCtx, cancel := context.WithTimeout(req.Context(), 120*time.Second)
	ctx = evalCtx
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, "POST", evaluateAnswerURL, bytes.NewBuffer(body))
	if err != nil {
		return errorResponse(req, fmt.Sprintf("failed to build request: %v", err))
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return errorResponse(req, fmt.Sprintf("evaluate_answer call failed: %v", err))
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return errorResponse(req, fmt.Sprintf("evaluate_answer returned %d: %s", resp.StatusCode, string(respBody)))
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
