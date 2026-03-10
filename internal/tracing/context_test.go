package tracing

import (
	"context"
	"testing"
)

func TestWithTraceID(t *testing.T) {
	ctx := context.Background()
	traceID := "test-trace-123"

	ctx = WithTraceID(ctx, traceID)
	got := GetTraceID(ctx)

	if got != traceID {
		t.Errorf("expected %s, got %s", traceID, got)
	}
}

func TestGetTraceID_Empty(t *testing.T) {
	ctx := context.Background()
	got := GetTraceID(ctx)

	if got != "" {
		t.Errorf("expected empty string, got %s", got)
	}
}

func TestGenerateTraceID(t *testing.T) {
	id1 := GenerateTraceID()
	id2 := GenerateTraceID()

	if id1 == "" {
		t.Error("expected non-empty trace ID")
	}
	if id1 == id2 {
		t.Error("expected unique trace IDs")
	}
}
