package planner

import (
	"testing"
)

func TestMakePlan_DefaultHybrid(t *testing.T) {
	plan := MakePlan("what is attention mechanism?", 5)

	if plan.Query != "what is attention mechanism?" {
		t.Errorf("expected query preserved, got %s", plan.Query)
	}

	// Harus ada minimal 2 steps: retrieve + evaluate
	if len(plan.Steps) < 2 {
		t.Errorf("expected at least 2 steps, got %d", len(plan.Steps))
	}

	// Step pertama harus retrieve_chunks
	if plan.Steps[0].ToolName != "retrieve_chunks" {
		t.Errorf("expected first step=retrieve_chunks, got %s", plan.Steps[0].ToolName)
	}

	// Method default harus hybrid
	if plan.Steps[0].Params["method"] != "hybrid" {
		t.Errorf("expected method=hybrid, got %v", plan.Steps[0].Params["method"])
	}

	// Step terakhir harus evaluate_answer
	last := plan.Steps[len(plan.Steps)-1]
	if last.ToolName != "evaluate_answer" {
		t.Errorf("expected last step=evaluate_answer, got %s", last.ToolName)
	}
}

func TestMakePlan_KeywordTriggersBM25(t *testing.T) {
	plan := MakePlan("exact keyword match for transformer", 5)

	if plan.Steps[0].ToolName != "retrieve_chunks" {
		t.Fatalf("expected retrieve_chunks first, got %s", plan.Steps[0].ToolName)
	}

	if plan.Steps[0].Params["method"] != "bm25" {
		t.Errorf("expected method=bm25 for 'exact' query, got %v", plan.Steps[0].Params["method"])
	}
}

func TestMakePlan_ArxivKeywordAddsSearchStep(t *testing.T) {
	testCases := []struct {
		query string
		desc  string
	}{
		{"search arxiv for transformers", "arxiv keyword"},
		{"find paper about BERT", "paper keyword"},
		{"research on attention mechanism", "research keyword"},
		{"study of RAG systems", "study keyword"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			plan := MakePlan(tc.query, 5)

			if plan.Steps[0].ToolName != "search_arxiv" {
				t.Errorf("[%s] expected first step=search_arxiv, got %s", tc.desc, plan.Steps[0].ToolName)
			}

			// Harus ada 3 steps: search + retrieve + evaluate
			if len(plan.Steps) != 3 {
				t.Errorf("[%s] expected 3 steps, got %d", tc.desc, len(plan.Steps))
			}
		})
	}
}

func TestMakePlan_TopKPropagated(t *testing.T) {
	plan := MakePlan("what is RAG?", 10)

	for _, step := range plan.Steps {
		if step.ToolName == "retrieve_chunks" {
			if step.Params["top_k"] != float64(10) {
				t.Errorf("expected top_k=10, got %v", step.Params["top_k"])
			}
		}
		if step.ToolName == "search_arxiv" {
			if step.Params["top_k"] != float64(10) {
				t.Errorf("expected top_k=10 in search_arxiv, got %v", step.Params["top_k"])
			}
		}
	}
}

func TestMakePlan_QueryPropagated(t *testing.T) {
	query := "transformer architecture explained"
	plan := MakePlan(query, 5)

	for _, step := range plan.Steps {
		if step.ToolName == "retrieve_chunks" {
			if step.Params["query"] != query {
				t.Errorf("expected query=%q in retrieve_chunks, got %v", query, step.Params["query"])
			}
		}
	}
}

func TestMakePlan_EvaluateAlwaysLast(t *testing.T) {
	queries := []string{
		"what is attention?",
		"arxiv paper on BERT",
		"exact keyword search",
		"research on transformers",
	}

	for _, q := range queries {
		plan := MakePlan(q, 5)
		last := plan.Steps[len(plan.Steps)-1]
		if last.ToolName != "evaluate_answer" {
			t.Errorf("query=%q: expected last step=evaluate_answer, got %s", q, last.ToolName)
		}
	}
}
