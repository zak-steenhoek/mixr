package mixr

import (
	"testing"
	"time"
)

func TestDiagnosticLimiterReportsFirstPeriodicAndRecovery(t *testing.T) {
	limiter := newDiagnosticLimiter(30 * time.Second)
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if !limiter.allow("mic", now) {
		t.Fatal("first warning was suppressed")
	}
	if limiter.allow("mic", now.Add(time.Second)) {
		t.Fatal("repeated warning was not suppressed")
	}
	if !limiter.allow("mic", now.Add(30*time.Second)) {
		t.Fatal("periodic warning was suppressed")
	}
	if !limiter.recovered("mic") {
		t.Fatal("recovery was not reported")
	}
	if limiter.recovered("mic") {
		t.Fatal("duplicate recovery was reported")
	}
}
