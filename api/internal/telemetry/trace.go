package telemetry

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"strings"
	"sync/atomic"
	"time"
)

type TraceContext struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	Sampled      bool
	TraceFlags   byte
}

func ParseTraceParent(raw string) (TraceContext, bool) {
	if len(raw) != 55 || raw[2] != '-' || raw[35] != '-' || raw[52] != '-' || raw[:2] != "00" || raw != strings.ToLower(raw) {
		return TraceContext{}, false
	}
	traceID, parentID := raw[3:35], raw[36:52]
	if !validLowerHex(traceID, 32) || !validLowerHex(parentID, 16) || !validLowerHex(raw[53:], 2) || allZero(traceID) || allZero(parentID) {
		return TraceContext{}, false
	}
	flags, _ := hex.DecodeString(raw[53:])
	return TraceContext{TraceID: traceID, ParentSpanID: parentID, SpanID: parentID, Sampled: flags[0]&1 == 1, TraceFlags: flags[0]}, true
}
func (t TraceContext) Header() string {
	if !validLowerHex(t.TraceID, 32) || allZero(t.TraceID) || !validLowerHex(t.SpanID, 16) || allZero(t.SpanID) {
		return ""
	}
	flagsValue := t.TraceFlags
	if t.Sampled {
		flagsValue |= 1
	} else {
		flagsValue &^= 1
	}
	flags := hex.EncodeToString([]byte{flagsValue})
	return "00-" + t.TraceID + "-" + t.SpanID + "-" + flags
}
func StartTrace(values []string, source io.Reader) TraceContext {
	if source == nil {
		source = rand.Reader
	}
	var result TraceContext
	if len(values) == 1 {
		if parent, ok := ParseTraceParent(values[0]); ok {
			result.TraceID, result.ParentSpanID, result.Sampled, result.TraceFlags = parent.TraceID, parent.ParentSpanID, parent.Sampled, parent.TraceFlags
		}
	}
	if result.TraceID == "" {
		result.TraceID = randomHex(source, 16)
	}
	result.SpanID = randomHex(source, 8)
	return result
}
func randomHex(source io.Reader, size int) string {
	value := make([]byte, size)
	for {
		if _, err := io.ReadFull(source, value); err != nil {
			value = fallbackRandom(size)
		}
		encoded := hex.EncodeToString(value)
		if !allZero(encoded) {
			return encoded
		}
	}
}

var fallbackSequence atomic.Uint64

func fallbackRandom(size int) []byte {
	var seed [16]byte
	binary.BigEndian.PutUint64(seed[:8], uint64(time.Now().UnixNano()))
	binary.BigEndian.PutUint64(seed[8:], fallbackSequence.Add(1))
	digest := sha256.Sum256(seed[:])
	value := make([]byte, size)
	copy(value, digest[:])
	return value
}
func validLowerHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, ch := range value {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return false
		}
	}
	return true
}
func allZero(value string) bool { return strings.Trim(value, "0") == "" }

type traceKey struct{}

func WithTrace(ctx context.Context, trace TraceContext) context.Context {
	correlation := CorrelationFromContext(ctx)
	correlation.TraceID, correlation.SpanID = trace.TraceID, trace.SpanID
	ctx = context.WithValue(ctx, correlationKey{}, correlation)
	return context.WithValue(ctx, traceKey{}, trace)
}
func TraceFromContext(ctx context.Context) TraceContext {
	value, _ := ctx.Value(traceKey{}).(TraceContext)
	return value
}
