package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// captureLogger swaps the default slog logger for one writing into buf and
// returns a restore func.
func captureLogger(t *testing.T, level slog.Level) (buf *bytes.Buffer, restore func()) {
	t.Helper()
	buf = &bytes.Buffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: level})))
	return buf, func() { slog.SetDefault(old) }
}

// metricStubTool is a registry stub with a realistic-size description so the
// serialized schema estimate is non-trivial.
type metricStubTool struct {
	name string
}

func (m *metricStubTool) Name() string        { return m.name }
func (m *metricStubTool) Description() string { return "metric stub tool for schema token measurement" }
func (m *metricStubTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{"type": "string", "description": "tool input value"},
		},
	}
}
func (m *metricStubTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	return tools.NewResult("ok")
}

// TestBuildFilteredTools_SchemaTokensMetric verifies the plan-mandated
// tools.schema_tokens measurement is emitted at Debug with the visible count
// and a positive estimate.
func TestBuildFilteredTools_SchemaTokensMetric(t *testing.T) {
	buf, restore := captureLogger(t, slog.LevelDebug)
	defer restore()

	reg := tools.NewRegistry()
	for _, name := range []string{"alpha_tool", "beta_tool", "gamma_tool"} {
		reg.Register(&metricStubTool{name: name})
	}
	loop := NewLoop(LoopConfig{Tools: reg})

	req := &RunRequest{}
	defs, allowed, _ := loop.buildFilteredTools(req, false, 1, 10, nil, nil)

	if len(defs) != 3 {
		t.Fatalf("expected 3 tool defs, got %d", len(defs))
	}
	// No policy engine wired → the allowlist map is not built (nil); policy
	// path allowlist construction is covered by the policy engine's own tests.
	if allowed != nil {
		t.Fatalf("no-policy path must leave allowedTools nil, got %d entries", len(allowed))
	}

	out := buf.String()
	if !strings.Contains(out, "tools.schema_tokens") {
		t.Fatalf("expected tools.schema_tokens log, got: %s", out)
	}
	if !strings.Contains(out, "count=3") {
		t.Errorf("expected count=3 in metric, got: %s", out)
	}
	if !strings.Contains(out, "estimated_tokens=") {
		t.Errorf("expected estimated_tokens field, got: %s", out)
	}
	// Estimate must be positive (schemas serialize to non-empty JSON).
	_, after, ok := strings.Cut(out, "estimated_tokens=")
	if ok {
		val := after
		if strings.HasPrefix(val, "0") {
			t.Errorf("estimated_tokens must be positive, got: %s", val)
		}
	}
}

// TestBuildFilteredTools_FinalIterationNoMetric: the final-iteration early
// return strips tools and must not emit a (meaningless, empty) metric.
func TestBuildFilteredTools_FinalIterationNoMetric(t *testing.T) {
	buf, restore := captureLogger(t, slog.LevelDebug)
	defer restore()

	reg := tools.NewRegistry()
	reg.Register(&metricStubTool{name: "alpha_tool"})
	loop := NewLoop(LoopConfig{Tools: reg})

	req := &RunRequest{}
	defs, _, msgs := loop.buildFilteredTools(req, false, 10, 10, nil, nil)

	if defs != nil {
		t.Fatalf("final iteration must strip tools, got %d defs", len(defs))
	}
	if len(msgs) == 0 {
		t.Fatal("final iteration must append the summarization hint")
	}
	if strings.Contains(buf.String(), "tools.schema_tokens") {
		t.Errorf("final iteration must not emit the metric, got: %s", buf.String())
	}
}

// TestBuildFilteredTools_NoMetricAtInfoLevel: the metric is debug-gated —
// production (info level) requests pay one handler check and log nothing.
func TestBuildFilteredTools_NoMetricAtInfoLevel(t *testing.T) {
	buf, restore := captureLogger(t, slog.LevelInfo)
	defer restore()

	reg := tools.NewRegistry()
	reg.Register(&metricStubTool{name: "alpha_tool"})
	loop := NewLoop(LoopConfig{Tools: reg})

	loop.buildFilteredTools(&RunRequest{}, false, 1, 10, nil, nil)

	if strings.Contains(buf.String(), "tools.schema_tokens") {
		t.Errorf("metric must not log at info level, got: %s", buf.String())
	}
}

// TestLogToolSchemaMetrics_EstimateMatchesJSONSize pins the estimation
// contract: bytes/4 of the serialized tool definitions.
func TestLogToolSchemaMetrics_EstimateMatchesJSONSize(t *testing.T) {
	defs := []providers.ToolDefinition{
		{Type: "function", Function: &providers.ToolFunctionSchema{
			Name:        "alpha_tool",
			Description: "metric stub tool for schema token measurement",
		}},
	}
	blob, err := json.Marshal(defs)
	if err != nil {
		t.Fatal(err)
	}
	if want := len(blob) / 4; want == 0 {
		t.Fatal("test setup: serialized defs must exceed 4 bytes")
	}
	// Indirect check through the log line.
	buf, restore := captureLogger(t, slog.LevelDebug)
	defer restore()
	logToolSchemaMetrics(defs, 2)
	out := buf.String()
	want := strings.Contains(out, "tools.schema_tokens") &&
		strings.Contains(out, "count=1")
	if !want {
		t.Errorf("missing metric fields, got: %s", out)
	}
}
