package health

import (
	"testing"
)

// BenchmarkRecord measures the per-event cost of telemetry recording on the
// request hot path. It is non-blocking (channel send); the aggregator goroutine
// is not started so this isolates the producer cost only.
func BenchmarkRecord(b *testing.B) {
	svc := New(Config{Enabled: true, QueueSize: 100_000}, nil, nil)
	ev := ProviderTelemetryEvent{
		Provider: "openai", Model: "gpt-4o", Capability: "chat_completions",
		Status: "success", LatencyMs: 1200, InputTokens: 100, OutputTokens: 50,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.Record(ev)
	}
}
