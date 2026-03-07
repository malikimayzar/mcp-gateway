package registry

import (
	"context"
	"testing"
)

func TestNew(t *testing.T) {
	r := New()
	if r == nil {
		t.Fatal("New() returned nil")
	}
	if len(r.List()) != 0 {
		t.Errorf("expected empty registry, got %d tools", len(r.List()))
	}
}

func TestRegisterAndList(t *testing.T) {
	r := New()

	r.Register("tool_a", func(ctx context.Context, req ToolRequest) ToolResponse {
		return ToolResponse{Success: true}
	})
	r.Register("tool_b", func(ctx context.Context, req ToolRequest) ToolResponse {
		return ToolResponse{Success: true}
	})

	tools := r.List()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}

	found := map[string]bool{}
	for _, name := range tools {
		found[name] = true
	}
	if !found["tool_a"] || !found["tool_b"] {
		t.Errorf("expected tool_a and tool_b in list, got %v", tools)
	}
}

func TestExecute_Success(t *testing.T) {
	r := New()
	r.Register("echo", func(ctx context.Context, req ToolRequest) ToolResponse {
		return ToolResponse{
			ToolName: req.ToolName,
			TraceID:  req.TraceID,
			Success:  true,
			Data:     map[string]interface{}{"echo": req.Params["input"]},
		}
	})

	resp := r.Execute(context.Background(), ToolRequest{
		ToolName: "echo",
		TraceID:  "trace-001",
		Params:   map[string]interface{}{"input": "hello"},
	})

	if !resp.Success {
		t.Errorf("expected success, got error: %s", resp.Error)
	}
	if resp.TraceID != "trace-001" {
		t.Errorf("expected trace-001, got %s", resp.TraceID)
	}
	if resp.Data["echo"] != "hello" {
		t.Errorf("expected echo=hello, got %v", resp.Data["echo"])
	}
}

func TestExecute_ToolNotFound(t *testing.T) {
	r := New()

	resp := r.Execute(context.Background(), ToolRequest{
		ToolName: "nonexistent",
		TraceID:  "trace-002",
	})

	if resp.Success {
		t.Error("expected failure for nonexistent tool")
	}
	if resp.Error == "" {
		t.Error("expected error message, got empty string")
	}
	if resp.ToolName != "nonexistent" {
		t.Errorf("expected tool_name=nonexistent, got %s", resp.ToolName)
	}
}

func TestExecute_ContextPropagation(t *testing.T) {
	r := New()

	type ctxKey string
	key := ctxKey("test-key")

	r.Register("ctx_check", func(ctx context.Context, req ToolRequest) ToolResponse {
		val, ok := ctx.Value(key).(string)
		if !ok || val != "test-value" {
			return ToolResponse{Success: false, Error: "context value not propagated"}
		}
		return ToolResponse{Success: true}
	})

	ctx := context.WithValue(context.Background(), key, "test-value")
	resp := r.Execute(ctx, ToolRequest{ToolName: "ctx_check"})

	if !resp.Success {
		t.Errorf("context not propagated: %s", resp.Error)
	}
}

func TestRegister_Overwrite(t *testing.T) {
	r := New()

	r.Register("tool", func(ctx context.Context, req ToolRequest) ToolResponse {
		return ToolResponse{Success: false, Error: "old handler"}
	})
	r.Register("tool", func(ctx context.Context, req ToolRequest) ToolResponse {
		return ToolResponse{Success: true}
	})

	// List harus tetap 1 tool
	if len(r.List()) != 1 {
		t.Errorf("expected 1 tool after overwrite, got %d", len(r.List()))
	}

	// Execute harus pakai handler baru
	resp := r.Execute(context.Background(), ToolRequest{ToolName: "tool"})
	if !resp.Success {
		t.Errorf("expected new handler to be used, got: %s", resp.Error)
	}
}