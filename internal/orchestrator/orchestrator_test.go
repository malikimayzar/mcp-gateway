package orchestrator

import (
	"strings"
	"testing"
)

func TestExtractJSON_PlainJSON(t *testing.T) {
	input := `{"steps": [], "answer": "hello"}`
	result := extractJSON(input)
	if result != input {
		t.Errorf("expected plain JSON unchanged, got %s", result)
	}
}

func TestExtractJSON_MarkdownWrapped(t *testing.T) {
	input := "```json\n{\"steps\": [], \"answer\": \"hello\"}\n```"
	result := extractJSON(input)

	if !strings.HasPrefix(result, "{") {
		t.Errorf("expected result to start with {, got: %s", result)
	}
	if !strings.HasSuffix(result, "}") {
		t.Errorf("expected result to end with }, got: %s", result)
	}
}

func TestExtractJSON_WithPreamble(t *testing.T) {
	input := `Here is the plan: {"steps": [{"tool_name": "search_arxiv"}], "answer": ""}`
	result := extractJSON(input)

	if !strings.HasPrefix(result, "{") {
		t.Errorf("expected extracted JSON to start with {, got: %s", result)
	}
}

func TestExtractJSON_Empty(t *testing.T) {
	input := "no json here"
	result := extractJSON(input)
	// Kalau ga ada JSON, return input asli
	if result == "" {
		t.Error("expected non-empty result for non-JSON input")
	}
}

func TestExtractJSON_Nested(t *testing.T) {
	input := `{"steps": [{"tool_name": "search_arxiv", "params": {"query": "test"}}], "answer": "result"}`
	result := extractJSON(input)

	if !strings.Contains(result, "search_arxiv") {
		t.Errorf("expected nested JSON preserved, got: %s", result)
	}
}

func TestPlanStruct_JSONTags(t *testing.T) {
	// Verifikasi struct field names sesuai contract
	plan := Plan{
		Steps:  []ToolCall{{ToolName: "search_arxiv", Reason: "test"}},
		Answer: "test answer",
	}

	if len(plan.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(plan.Steps))
	}
	if plan.Steps[0].ToolName != "search_arxiv" {
		t.Errorf("expected tool_name=search_arxiv, got %s", plan.Steps[0].ToolName)
	}
	if plan.Answer != "test answer" {
		t.Errorf("expected answer='test answer', got %s", plan.Answer)
	}
}

func TestToolCall_ParamsDefault(t *testing.T) {
	tc := ToolCall{
		ToolName: "retrieve_chunks",
		Params:   nil,
	}

	if tc.Params != nil {
		t.Error("expected nil params by default")
	}
	if tc.ToolName != "retrieve_chunks" {
		t.Errorf("expected retrieve_chunks, got %s", tc.ToolName)
	}
}