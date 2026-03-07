package store

import (
	"testing"
)

func TestExtractFailureMode_WithFailureMode(t *testing.T) {
	data := map[string]interface{}{
		"failure_mode":       "unsupported_claims",
		"faithfulness_score": 0.5,
	}

	result := ExtractFailureMode(data)
	if result != "unsupported_claims" {
		t.Errorf("expected unsupported_claims, got %s", result)
	}
}

func TestExtractFailureMode_NilData(t *testing.T) {
	result := ExtractFailureMode(nil)
	if result != "" {
		t.Errorf("expected empty string for nil data, got %s", result)
	}
}

func TestExtractFailureMode_MissingKey(t *testing.T) {
	data := map[string]interface{}{
		"faithfulness_score": 0.8,
	}

	result := ExtractFailureMode(data)
	if result != "" {
		t.Errorf("expected empty string for missing key, got %s", result)
	}
}

func TestExtractFailureMode_WrongType(t *testing.T) {
	data := map[string]interface{}{
		"failure_mode": 123, // wrong type, bukan string
	}

	result := ExtractFailureMode(data)
	if result != "" {
		t.Errorf("expected empty string for wrong type, got %s", result)
	}
}

func TestExtractFailureMode_EmptyString(t *testing.T) {
	data := map[string]interface{}{
		"failure_mode": "",
	}

	result := ExtractFailureMode(data)
	if result != "" {
		t.Errorf("expected empty string, got %s", result)
	}
}

func TestExtractFailureMode_AllKnownModes(t *testing.T) {
	modes := []string{
		"unsupported_claims",
		"insufficient_context",
		"none",
	}

	for _, mode := range modes {
		data := map[string]interface{}{"failure_mode": mode}
		result := ExtractFailureMode(data)
		if result != mode {
			t.Errorf("expected %s, got %s", mode, result)
		}
	}
}
