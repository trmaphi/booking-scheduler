package telemetry_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"scheduler/api/internal/telemetry"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("entropy unavailable") }

func TestTraceParentStrictParsing(t *testing.T) {
	valid := "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"
	parsed, ok := telemetry.ParseTraceParent(valid)
	if !ok || parsed.TraceID != "0123456789abcdef0123456789abcdef" || parsed.ParentSpanID != "0123456789abcdef" || !parsed.Sampled {
		t.Fatalf("parsed = %#v, %v", parsed, ok)
	}
	if parsed.Header() != valid {
		t.Fatalf("header = %q", parsed.Header())
	}
	invalid := []string{"", "00-00000000000000000000000000000000-0123456789abcdef-01", "00-0123456789abcdef0123456789abcdef-0000000000000000-01", "01-0123456789abcdef0123456789abcdef-0123456789abcdef-01", "00-0123456789ABCDEf0123456789abcdef-0123456789abcdef-01", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01-extra", "00-xyz-0123456789abcdef-01"}
	for _, raw := range invalid {
		if _, ok := telemetry.ParseTraceParent(raw); ok {
			t.Errorf("accepted %q", raw)
		}
	}
}

func TestTraceParentAcceptsAndPreservesAllVersionZeroFlags(t *testing.T) {
	for _, tt := range []struct {
		flags   string
		sampled bool
	}{{"02", false}, {"03", true}, {"fe", false}, {"ff", true}} {
		raw := "00-0123456789abcdef0123456789abcdef-0123456789abcdef-" + tt.flags
		parsed, ok := telemetry.ParseTraceParent(raw)
		if !ok || parsed.Sampled != tt.sampled || parsed.Header() != raw {
			t.Errorf("flags %s parsed as %#v, %v header %q", tt.flags, parsed, ok, parsed.Header())
		}
		child := telemetry.StartTrace([]string{raw}, bytes.NewReader(bytes.Repeat([]byte{0x44}, 8)))
		if child.TraceID != parsed.TraceID || child.Sampled != tt.sampled || child.Header()[53:] != tt.flags {
			t.Errorf("child flags %s = %#v (%q)", tt.flags, child, child.Header())
		}
	}
}

func TestStartTracePreservesTraceAndCreatesUniqueChildren(t *testing.T) {
	parent := "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"
	random := bytes.NewReader(append(bytes.Repeat([]byte{0x11}, 8), bytes.Repeat([]byte{0x22}, 8)...))
	first := telemetry.StartTrace([]string{parent}, random)
	second := telemetry.StartTrace([]string{parent}, random)
	if first.TraceID != "0123456789abcdef0123456789abcdef" || first.SpanID == first.ParentSpanID || !first.Sampled {
		t.Fatalf("first = %#v", first)
	}
	if first.SpanID == second.SpanID {
		t.Fatalf("child spans are equal: %s", first.SpanID)
	}
	if first.Header() != "00-0123456789abcdef0123456789abcdef-1111111111111111-01" {
		t.Fatalf("header = %q", first.Header())
	}
}

func TestStartTraceReplacesMissingInvalidAndAmbiguousInput(t *testing.T) {
	for _, values := range [][]string{nil, {"invalid"}, {"00-0123456789abcdef0123456789abcdef-0123456789abcdef-01", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"}} {
		trace := telemetry.StartTrace(values, bytes.NewReader(bytes.Repeat([]byte{0x33}, 24)))
		if trace.TraceID != "33333333333333333333333333333333" || trace.SpanID != "3333333333333333" || trace.ParentSpanID != "" || trace.Sampled {
			t.Fatalf("trace = %#v", trace)
		}
	}
}

func TestTraceContextCarriesSpanCorrelation(t *testing.T) {
	trace := telemetry.TraceContext{TraceID: "0123456789abcdef0123456789abcdef", SpanID: "1111111111111111", ParentSpanID: "2222222222222222", Sampled: true}
	ctx := telemetry.WithTrace(context.Background(), trace)
	if got := telemetry.TraceFromContext(ctx); got != trace {
		t.Fatalf("trace = %#v", got)
	}
}

func TestStartTraceEntropyFailureStillReturnsValidContext(t *testing.T) {
	trace := telemetry.StartTrace(nil, failingReader{})
	if _, ok := telemetry.ParseTraceParent(trace.Header()); !ok {
		t.Fatalf("fallback trace is invalid: %#v", trace)
	}
	if _, err := io.WriteString(io.Discard, trace.Header()); err != nil {
		t.Fatal(err)
	}
}
