package facebook

import (
	"net/http"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter()
	state := rl.GetState("unknown-token")
	if state.IsBlocked() {
		t.Error("new token should not be blocked")
	}
	if state.AppUsagePct != 0 || state.BDCUsagePct != 0 || state.InsightsPct != 0 {
		t.Error("new token should have zero usage")
	}
}

func TestUpdateFromResponse_AppUsage(t *testing.T) {
	rl := NewRateLimiter()
	resp := &http.Response{
		Header: http.Header{
			"X-App-Usage": {`{"call_count":60,"total_cputime":20,"total_time":10}`},
		},
	}
	rl.UpdateFromResponse("tok1", resp)

	state := rl.GetState("tok1")
	// UsagePct should be max(60, 20, 10) = 60
	if state.AppUsagePct != 60 {
		t.Errorf("AppUsagePct = %v, want 60", state.AppUsagePct)
	}
	if state.UsagePct() != 60 {
		t.Errorf("UsagePct() = %v, want 60", state.UsagePct())
	}
}

func TestUpdateFromResponse_BDCUsage(t *testing.T) {
	rl := NewRateLimiter()
	resp := &http.Response{
		Header: http.Header{
			"X-Business-Use-Case-Usage": {`{"bm_123":[{"type":"ads_management","call_count":45,"total_cputime":10,"total_time":5,"estimated_time_to_regain_access":0}]}`},
		},
	}
	rl.UpdateFromResponse("tok2", resp)

	state := rl.GetState("tok2")
	if state.BDCUsagePct != 45 {
		t.Errorf("BDCUsagePct = %v, want 45", state.BDCUsagePct)
	}
}

func TestUpdateFromResponse_InsightsThrottle(t *testing.T) {
	rl := NewRateLimiter()
	h := make(http.Header)
	h.Set("X-FB-Ads-Insights-Throttle", `{"app_id_util_pct":30,"acc_id_util_pct":55}`)
	resp := &http.Response{Header: h}
	rl.UpdateFromResponse("tok3", resp)

	state := rl.GetState("tok3")
	// max(30, 55) = 55
	if state.InsightsPct != 55 {
		t.Errorf("InsightsPct = %v, want 55", state.InsightsPct)
	}
}

func TestUpdateFromResponse_BlockedByEstimate(t *testing.T) {
	rl := NewRateLimiter()
	resp := &http.Response{
		Header: http.Header{
			"X-Business-Use-Case-Usage": {`{"bm_1":[{"type":"ads_insights","call_count":100,"total_cputime":100,"total_time":100,"estimated_time_to_regain_access":5}]}`},
		},
	}
	rl.UpdateFromResponse("tok4", resp)

	state := rl.GetState("tok4")
	if !state.IsBlocked() {
		t.Error("token should be blocked when estimated_time_to_regain_access > 0")
	}
}

func TestUpdateFromResponse_EmptyHeaders(t *testing.T) {
	rl := NewRateLimiter()
	rl.UpdateFromResponse("tok5", &http.Response{Header: http.Header{}})
	state := rl.GetState("tok5")
	if state.IsBlocked() || state.AppUsagePct != 0 {
		t.Error("empty headers should produce zero state")
	}
}

func TestBlockToken(t *testing.T) {
	rl := NewRateLimiter()

	initial := rl.GetState("tok")
	if initial.IsBlocked() {
		t.Fatal("should not be blocked initially")
	}

	rl.BlockToken("tok", 10*time.Minute)

	state := rl.GetState("tok")
	if !state.IsBlocked() {
		t.Error("token should be blocked after BlockToken")
	}
	if state.BlockedUntil.Before(time.Now().Add(9 * time.Minute)) {
		t.Error("BlockedUntil should be ~10 minutes from now")
	}
}

func TestBlockToken_AlreadyHasState(t *testing.T) {
	rl := NewRateLimiter()
	resp := &http.Response{
		Header: http.Header{
			"X-App-Usage": {`{"call_count":50,"total_cputime":0,"total_time":0}`},
		},
	}
	rl.UpdateFromResponse("tok", resp)
	rl.BlockToken("tok", 5*time.Minute)

	state := rl.GetState("tok")
	// AppUsagePct should be preserved after BlockToken on existing state.
	// Note: BlockToken replaces the state's BlockedUntil but UpdateFromResponse
	// resets the whole state, so the order here is: update then block.
	if !state.IsBlocked() {
		t.Error("token should be blocked")
	}
}

func TestQuotaState_UsagePct(t *testing.T) {
	q := QuotaState{AppUsagePct: 30, BDCUsagePct: 70, InsightsPct: 50}
	if q.UsagePct() != 70 {
		t.Errorf("UsagePct() = %v, want 70", q.UsagePct())
	}
}

func TestGetState_Concurrent(t *testing.T) {
	rl := NewRateLimiter()
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(id string) {
			rl.BlockToken(id, time.Minute)
			_ = rl.GetState(id)
			done <- struct{}{}
		}(string(rune('a' + i)))
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
