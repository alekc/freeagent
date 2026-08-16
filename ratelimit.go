package freeagent

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

// FreeAgent enforces 120 requests per minute and 3600 per hour per end user.
// The defaults below sit under both: the token buckets refill continuously
// while the API counters reset on fixed boundaries, so burst plus refill must
// still fit inside a single window.
const (
	DefaultRequestsPerMinute = 100
	DefaultRequestsPerHour   = 3400

	minuteBurst = 10
	hourBurst   = 100
)

// RateLimit reports the server's own accounting, when it sends it. FreeAgent
// documents only Retry-After, so these fields are best effort and a zero
// value means the response carried no such header.
type RateLimit struct {
	Limit     int
	Remaining int
	Reset     time.Time
}

// limiterSet holds the per-minute and per-hour budgets. Both must admit a
// request before it is sent; over a long run the hourly budget dominates.
type limiterSet struct {
	minute *rate.Limiter
	hour   *rate.Limiter
}

func newLimiterSet(perMinute, perHour int) *limiterSet {
	set := &limiterSet{}
	if perMinute > 0 {
		set.minute = rate.NewLimiter(rate.Limit(float64(perMinute)/60.0), burstFor(perMinute, minuteBurst))
	}
	if perHour > 0 {
		set.hour = rate.NewLimiter(rate.Limit(float64(perHour)/3600.0), burstFor(perHour, hourBurst))
	}
	return set
}

func burstFor(budget, want int) int {
	if budget < want {
		return budget
	}
	return want
}

// wait blocks until both budgets admit one request, or ctx is done.
func (l *limiterSet) wait(ctx context.Context) error {
	if l == nil {
		return nil
	}
	if l.minute != nil {
		if err := l.minute.Wait(ctx); err != nil {
			return err
		}
	}
	if l.hour != nil {
		if err := l.hour.Wait(ctx); err != nil {
			return err
		}
	}
	return nil
}

func parseRateLimit(h http.Header) RateLimit {
	var rl RateLimit
	if v := h.Get("X-RateLimit-Limit"); v != "" {
		rl.Limit, _ = strconv.Atoi(v)
	}
	if v := h.Get("X-RateLimit-Remaining"); v != "" {
		rl.Remaining, _ = strconv.Atoi(v)
	}
	if v := h.Get("X-RateLimit-Reset"); v != "" {
		if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
			rl.Reset = time.Unix(secs, 0).UTC()
		}
	}
	return rl
}
