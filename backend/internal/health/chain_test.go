package health

import (
	"testing"
	"time"
)

// TestChainStats_Counting verifies the terminal-event counting model: a
// request through a chain emits one terminal event (success or final-failure)
// plus zero or more non-terminal fallback-failure events. Only terminal
// events count as requests; FallbackTriggered on a terminal event means the
// request fell back >=1 time.
func TestChainStats_Counting(t *testing.T) {
	svc := New(Config{Enabled: true, RollingWindow: time.Hour}, nil, nil)
	defer svc.Close(time.Second)

	now := time.Now()
	// Chain "coding": 2 successful requests, 1 request that fell back twice
	// then succeeded, 1 request that failed all attempts.
	svc.ingest(ProviderTelemetryEvent{Timestamp: now, ChainID: "coding", Status: "success"})
	svc.ingest(ProviderTelemetryEvent{Timestamp: now, ChainID: "coding", Status: "success"})
	// request 3: two non-terminal fallback failures then success (fellBack=true)
	svc.ingest(ProviderTelemetryEvent{Timestamp: now, ChainID: "coding", Status: "failed", FallbackTriggered: true})
	svc.ingest(ProviderTelemetryEvent{Timestamp: now, ChainID: "coding", Status: "failed", FallbackTriggered: true})
	svc.ingest(ProviderTelemetryEvent{Timestamp: now, ChainID: "coding", Status: "success", FallbackTriggered: true})
	// request 4: one non-terminal fallback failure then final failure (fellBack=true)
	svc.ingest(ProviderTelemetryEvent{Timestamp: now, ChainID: "coding", Status: "failed", FallbackTriggered: true})
	svc.ingest(ProviderTelemetryEvent{Timestamp: now, ChainID: "coding", Status: "failed", FinalFailure: true, FallbackTriggered: true})

	stats := svc.ChainStats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(stats))
	}
	st := stats[0]
	if st.ChainID != "coding" {
		t.Fatalf("chain id = %s", st.ChainID)
	}
	if st.Requests != 4 {
		t.Errorf("requests = %d, want 4", st.Requests)
	}
	if st.Successes != 3 {
		t.Errorf("successes = %d, want 3", st.Successes)
	}
	if st.FinalFailures != 1 {
		t.Errorf("final failures = %d, want 1", st.FinalFailures)
	}
	if st.Fallbacks != 2 {
		t.Errorf("fallbacks = %d, want 2", st.Fallbacks)
	}
	wantRate := 2.0 / 4.0
	if st.FallbackRate-wantRate > 1e-9 || wantRate-st.FallbackRate > 1e-9 {
		t.Errorf("fallback rate = %v, want %v", st.FallbackRate, wantRate)
	}
}
