package freeagent

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestLimiterSetDisabled(t *testing.T) {
	t.Parallel()
	var nilSet *limiterSet
	if err := nilSet.wait(context.Background()); err != nil {
		t.Fatalf("nil limiterSet.wait = %v, want nil", err)
	}
	if err := newLimiterSet(0, 0).wait(context.Background()); err != nil {
		t.Fatalf("disabled limiterSet.wait = %v, want nil", err)
	}
}

// The minute and hour budgets are independent, and either one exhausting
// must block the request rather than let it through.
func TestLimiterSetBlocksWhenBudgetExhausted(t *testing.T) {
	t.Parallel()
	tests := map[string]*limiterSet{
		"minute budget": newLimiterSet(1, 0),
		"hour budget":   newLimiterSet(0, 1),
	}
	for name, set := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// The first request consumes the single burst token.
			if err := set.wait(context.Background()); err != nil {
				t.Fatalf("first wait = %v, want nil", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if err := set.wait(ctx); err == nil {
				t.Fatal("second wait succeeded, want it to block until the deadline")
			}
		})
	}
}

// Burst must never exceed the budget itself, or a tiny budget would admit
// more requests in one window than the API allows.
func TestBurstForClampsToBudget(t *testing.T) {
	t.Parallel()
	tests := []struct{ budget, want, wanted int }{
		{budget: 3, wanted: 10, want: 3},
		{budget: 100, wanted: 10, want: 10},
		{budget: 10, wanted: 10, want: 10},
	}
	for _, tc := range tests {
		if got := burstFor(tc.budget, tc.wanted); got != tc.want {
			t.Fatalf("burstFor(%d, %d) = %d, want %d", tc.budget, tc.wanted, got, tc.want)
		}
	}
}

func TestDefaultBudgetsStayUnderTheAPICaps(t *testing.T) {
	t.Parallel()
	// The API resets counters on fixed boundaries while a token bucket
	// refills continuously, so the worst case in one window is burst plus a
	// full window of refill. Both must stay under the published caps.
	if worst := DefaultRequestsPerMinute + minuteBurst; worst > 120 {
		t.Fatalf("worst case per minute = %d, want at most 120", worst)
	}
	if worst := DefaultRequestsPerHour + hourBurst; worst > 3600 {
		t.Fatalf("worst case per hour = %d, want at most 3600", worst)
	}
}

func TestParseRateLimitHeaders(t *testing.T) {
	t.Parallel()
	got := parseRateLimit(http.Header{
		"X-Ratelimit-Limit":     {"120"},
		"X-Ratelimit-Remaining": {"7"},
		"X-Ratelimit-Reset":     {"1774000000"},
	})
	if got.Limit != 120 || got.Remaining != 7 {
		t.Fatalf("limit/remaining = %d/%d, want 120/7", got.Limit, got.Remaining)
	}
	if want := time.Unix(1774000000, 0).UTC(); !got.Reset.Equal(want) {
		t.Fatalf("Reset = %s, want %s", got.Reset, want)
	}

	// FreeAgent documents only Retry-After, so absent headers are normal and
	// must yield a zero value rather than an error.
	empty := parseRateLimit(http.Header{})
	if empty.Limit != 0 || empty.Remaining != 0 || !empty.Reset.IsZero() {
		t.Fatalf("empty headers = %+v, want a zero RateLimit", empty)
	}

	junk := parseRateLimit(http.Header{"X-Ratelimit-Limit": {"lots"}, "X-Ratelimit-Reset": {"soon"}})
	if junk.Limit != 0 || !junk.Reset.IsZero() {
		t.Fatalf("unparsable headers = %+v, want a zero RateLimit", junk)
	}
}
