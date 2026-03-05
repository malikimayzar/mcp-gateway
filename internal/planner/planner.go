package planner

import "strings"

type Step struct {
	ToolName string
	Params   map[string]interface{}
}

type Plan struct {
	Query string
	Steps []Step
}

func MakePlan(query string, topK int) Plan {
	q := strings.ToLower(query)
	steps := []Step{}

	// Step 1: selalu retrieve dulu
	method := "hybrid"
	if strings.Contains(q, "exact") || strings.Contains(q, "keyword") {
		method = "bm25"
	}
	steps = append(steps, Step{
		ToolName: "retrieve_chunks",
		Params: map[string]interface{}{
			"query":  query,
			"top_k":  float64(topK),
			"method": method,
		},
	})

	// Step 2: kalau ada kata arxiv atau paper, search dulu
	if strings.Contains(q, "arxiv") || strings.Contains(q, "paper") ||
		strings.Contains(q, "research") || strings.Contains(q, "study") {
		steps = append([]Step{{
			ToolName: "search_arxiv",
			Params: map[string]interface{}{
				"query": query,
				"top_k": float64(topK),
			},
		}}, steps...)
	}

	// Step 3: selalu evaluate di akhir
	steps = append(steps, Step{
		ToolName: "evaluate_answer",
		Params:   map[string]interface{}{},
	})

	return Plan{Query: query, Steps: steps}
}
