package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

type ToolCall struct {
	ToolName string                 `json:"tool_name"`
	Params   map[string]interface{} `json:"params"`
	Reason   string                 `json:"reason"`
}

type OrchestratorPlan struct {
	Steps  []ToolCall `json:"steps"`
	Answer string     `json:"answer"`
}

var systemPrompt = `You are an AI research assistant orchestrator. Given a user query, decide which tools to call and in what order.

Available tools:
1. search_arxiv — search and query ArXiv papers using RAG. Use for research questions about ML/AI papers.
2. retrieve_chunks — retrieve relevant text chunks from local document index. Use for quick factual lookups.
3. evaluate_answer — evaluate faithfulness of an answer against context. Always use this as the last step.

Respond ONLY with valid JSON (no markdown, no code blocks) in this exact format:
{
  "steps": [
    {"tool_name": "retrieve_chunks", "params": {"query": "...", "top_k": 5, "method": "hybrid"}, "reason": "..."},
    {"tool_name": "evaluate_answer", "params": {}, "reason": "evaluate faithfulness"}
  ],
  "answer": ""
}`

func extractJSON(raw string) string {
	raw = strings.TrimSpace(raw)

	// Cari { pertama dan } terakhir — works untuk plain JSON maupun wrapped markdown
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start != -1 && end != -1 && end > start {
		return strings.TrimSpace(raw[start : end+1])
	}
	return raw
}

func Plan(ctx context.Context, query string) (OrchestratorPlan, error) {
	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		return OrchestratorPlan{}, fmt.Errorf("GROQ_API_KEY not set")
	}

	config := openai.DefaultConfig(apiKey)
	config.BaseURL = "https://api.groq.com/openai/v1"
	client := openai.NewClientWithConfig(config)

	log.Printf("[groq] calling Groq LLM for query: %q", query)

	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: "llama-3.3-70b-versatile",
		Messages: []openai.ChatCompletionMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: query},
		},
		Temperature: 0.1,
	})
	if err != nil {
		log.Printf("[groq] API call failed: %v", err)
		return OrchestratorPlan{}, fmt.Errorf("groq call failed: %w", err)
	}

	raw := resp.Choices[0].Message.Content
	log.Printf("[groq] raw response: %s", raw)

	cleaned := extractJSON(raw)
	log.Printf("[groq] cleaned JSON: %s", cleaned)

	var plan OrchestratorPlan
	if err := json.Unmarshal([]byte(cleaned), &plan); err != nil {
		log.Printf("[groq] JSON parse failed: %v", err)
		return OrchestratorPlan{}, fmt.Errorf("failed to parse groq response: %w\nraw: %s", err, raw)
	}

	log.Printf("[groq] plan parsed OK — %d steps", len(plan.Steps))
	for i, step := range plan.Steps {
		log.Printf("[groq]   step[%d]: tool=%s reason=%q", i, step.ToolName, step.Reason)
	}

	return plan, nil
}
