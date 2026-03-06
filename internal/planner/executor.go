package planner

import (
	"context"
	"fmt"
	"strings"

	"github.com/malikimayzar/mcp-gateway/internal/registry"
)

type ExecutionResult struct {
	Query   string
	Steps   []StepResult
	Answer  string
	Context string
	Score   float64
	Retried bool
}

type StepResult struct {
	ToolName string
	Success  bool
	Error    string
	Data     map[string]interface{}
}

func Execute(ctx context.Context, reg *registry.Registry, plan Plan) ExecutionResult {
	result := ExecutionResult{Query: plan.Query}
	var answer string
	var contextStr string

	for _, step := range plan.Steps {
		req := registry.ToolRequest{
			ToolName: step.ToolName,
			Params:   step.Params,
			TraceID:  fmt.Sprintf("plan-%s", plan.Query[:min(10, len(plan.Query))]),
		}

		// inject context dari step sebelumnya ke evaluate_answer
		if step.ToolName == "evaluate_answer" {
			if answer == "" || contextStr == "" {
				result.Steps = append(result.Steps, StepResult{
					ToolName: step.ToolName,
					Success:  false,
					Error:    "skipped: no answer or context available",
				})
				continue
			}
			req.Params["answer"] = answer
			req.Params["context"] = contextStr
			req.Params["question"] = plan.Query
		}

		resp := reg.Execute(ctx, req)
		stepResult := StepResult{
			ToolName: step.ToolName,
			Success:  resp.Success,
			Error:    resp.Error,
			Data:     resp.Data,
		}
		result.Steps = append(result.Steps, stepResult)

		if !resp.Success {
			continue
		}

		// extract answer dan context dari hasil tool
		if step.ToolName == "search_arxiv" {
			if a, ok := resp.Data["answer"].(string); ok {
				answer = a
			}
			if sources, ok := resp.Data["sources"].([]interface{}); ok {
				var parts []string
				for _, s := range sources {
					if sm, ok := s.(map[string]interface{}); ok {
						if t, ok := sm["text"].(string); ok {
							parts = append(parts, t)
						}
					}
				}
				contextStr = strings.Join(parts, " ")
			}
		}

		if step.ToolName == "retrieve_chunks" {
			if results, ok := resp.Data["results"].([]interface{}); ok {
				var parts []string
				for _, r := range results {
					if rm, ok := r.(map[string]interface{}); ok {
						if t, ok := rm["text"].(string); ok {
							parts = append(parts, t)
						}
					}
				}
				contextStr = strings.Join(parts, " ")
				if answer == "" {
					answer = contextStr
				}
			}
		}

		if step.ToolName == "evaluate_answer" {
			if score, ok := resp.Data["faithfulness_score"].(float64); ok {
				result.Score = score
			}
		}
	}

	result.Answer = answer
	result.Context = contextStr
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func ExecuteWithRetry(ctx context.Context, reg *registry.Registry, query string, topK int) ExecutionResult {
	// First attempt: hybrid
	plan := MakePlan(query, topK)
	result := Execute(ctx, reg, plan)

	// Kalau score rendah, retry dengan bm25
	if result.Score < 0.6 && result.Score > 0 {
		retryPlan := Plan{
			Query: query,
			Steps: []Step{
				{
					ToolName: "retrieve_chunks",
					Params: map[string]interface{}{
						"query":  query,
						"top_k":  float64(topK),
						"method": "bm25",
					},
				},
				{
					ToolName: "evaluate_answer",
					Params:   map[string]interface{}{},
				},
			},
		}
		retryResult := Execute(ctx, reg, retryPlan)
		if retryResult.Score > result.Score {
			retryResult.Query = query
			retryResult.Retried = true
			return retryResult
		}
	}

	return result
}
