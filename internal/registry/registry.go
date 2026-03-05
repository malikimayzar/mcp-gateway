package registry

import (
	"context"
	"fmt"
)

type ToolRequest struct {
	ToolName string                 `json:"tool_name"`
	Params   map[string]interface{} `json:"params"`
	TraceID  string                 `json:"trace_id"`
}

type ToolResponse struct {
	ToolName string                 `json:"tool_name"`
	TraceID  string                 `json:"trace_id"`
	Success  bool                   `json:"success"`
	Data     map[string]interface{} `json:"data,omitempty"`
	Error    string                 `json:"error,omitempty"`
}

type ToolHandler func(ctx context.Context, req ToolRequest) ToolResponse

type Registry struct {
	tools map[string]ToolHandler
}

func New() *Registry {
	return &Registry{tools: make(map[string]ToolHandler)}
}

func (r *Registry) Register(name string, handler ToolHandler) {
	r.tools[name] = handler
}

func (r *Registry) Execute(ctx context.Context, req ToolRequest) ToolResponse {
	handler, ok := r.tools[req.ToolName]
	if !ok {
		return ToolResponse{
			ToolName: req.ToolName,
			TraceID:  req.TraceID,
			Success:  false,
			Error:    fmt.Sprintf("tool not found: %s", req.ToolName),
		}
	}
	return handler(ctx, req)
}

func (r *Registry) List() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}