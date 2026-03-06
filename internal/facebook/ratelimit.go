package facebook

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// QuotaState tracks current rate limit usage for one token.
type QuotaState struct {
	AppUsagePct     float64
	BDCUsagePct     float64
	InsightsPct     float64
	BlockedUntil    time.Time
	EstBlockMinutes int
}

// IsBlocked reports whether the token is currently blocked.
func (q *QuotaState) IsBlocked() bool {
	return time.Now().Before(q.BlockedUntil)
}

// UsagePct returns the highest usage percentage across all dimensions.
func (q *QuotaState) UsagePct() float64 {
	max := q.AppUsagePct
	if q.BDCUsagePct > max {
		max = q.BDCUsagePct
	}
	if q.InsightsPct > max {
		max = q.InsightsPct
	}
	return max
}

// RateLimiter tracks quota state per token (in-memory).
type RateLimiter struct {
	mu     sync.RWMutex
	states map[string]*QuotaState // key: token ID
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{states: make(map[string]*QuotaState)}
}

// UpdateFromResponse parses FB rate limit headers from an HTTP response.
func (rl *RateLimiter) UpdateFromResponse(tokenID string, resp *http.Response) {
	state := &QuotaState{}

	// X-App-Usage: {"call_count":5,"total_cputime":2,"total_time":2}
	if v := resp.Header.Get("X-App-Usage"); v != "" {
		var u struct {
			CallCount   float64 `json:"call_count"`
			TotalCPU    float64 `json:"total_cputime"`
			TotalTime   float64 `json:"total_time"`
		}
		if json.Unmarshal([]byte(v), &u) == nil {
			state.AppUsagePct = maxFloat(u.CallCount, u.TotalCPU, u.TotalTime)
		}
	}

	// X-Business-Use-Case-Usage: {"bm_id":[{"type":"ads_insights","call_count":12,...}]}
	if v := resp.Header.Get("X-Business-Use-Case-Usage"); v != "" {
		var bm map[string][]struct {
			Type                      string  `json:"type"`
			CallCount                 float64 `json:"call_count"`
			TotalCPU                  float64 `json:"total_cputime"`
			TotalTime                 float64 `json:"total_time"`
			EstimatedTimeToRegainAccess int   `json:"estimated_time_to_regain_access"`
		}
		if json.Unmarshal([]byte(v), &bm) == nil {
			for _, entries := range bm {
				for _, e := range entries {
					pct := maxFloat(e.CallCount, e.TotalCPU, e.TotalTime)
					if pct > state.BDCUsagePct {
						state.BDCUsagePct = pct
					}
					if e.EstimatedTimeToRegainAccess > state.EstBlockMinutes {
						state.EstBlockMinutes = e.EstimatedTimeToRegainAccess
					}
				}
			}
		}
	}

	// X-FB-Ads-Insights-Throttle: {"app_id_util_pct":12,"acc_id_util_pct":8}
	if v := resp.Header.Get("X-FB-Ads-Insights-Throttle"); v != "" {
		var t struct {
			AppPct float64 `json:"app_id_util_pct"`
			AccPct float64 `json:"acc_id_util_pct"`
		}
		if json.Unmarshal([]byte(v), &t) == nil {
			state.InsightsPct = maxFloat(t.AppPct, t.AccPct)
		}
	}

	if state.EstBlockMinutes > 0 {
		state.BlockedUntil = time.Now().Add(time.Duration(state.EstBlockMinutes) * time.Minute)
	}

	rl.mu.Lock()
	rl.states[tokenID] = state
	rl.mu.Unlock()
}

// BlockToken marks a token as blocked for the given duration.
func (rl *RateLimiter) BlockToken(tokenID string, d time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	s := rl.states[tokenID]
	if s == nil {
		s = &QuotaState{}
		rl.states[tokenID] = s
	}
	s.BlockedUntil = time.Now().Add(d)
}

// GetState returns a copy of the quota state for the given token.
func (rl *RateLimiter) GetState(tokenID string) QuotaState {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	if s := rl.states[tokenID]; s != nil {
		return *s
	}
	return QuotaState{}
}

func maxFloat(vals ...float64) float64 {
	var m float64
	for _, v := range vals {
		if v > m {
			m = v
		}
	}
	return m
}
