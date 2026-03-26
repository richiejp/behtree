package galcheck

import (
	"testing"
	"time"
)

func TestParseRateLimits_MultiBucket(t *testing.T) {
	now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	header := `"api";r=950;t=120, "resolvers";r=4800;t=120`

	api, resolvers := parseRateLimits(header, now)

	if api == nil {
		t.Fatal("expected api bucket, got nil")
	}
	if api.Remaining != 950 {
		t.Errorf("api.Remaining = %d, want 950", api.Remaining)
	}
	if api.ResetAt != now.Add(120*time.Second) {
		t.Errorf("api.ResetAt = %v, want %v", api.ResetAt, now.Add(120*time.Second))
	}

	if resolvers == nil {
		t.Fatal("expected resolvers bucket, got nil")
	}
	if resolvers.Remaining != 4800 {
		t.Errorf("resolvers.Remaining = %d, want 4800", resolvers.Remaining)
	}
}

func TestParseRateLimits_SingleBucket(t *testing.T) {
	now := time.Now()
	header := `"api";r=0;t=300`

	api, resolvers := parseRateLimits(header, now)

	if api == nil {
		t.Fatal("expected api bucket, got nil")
	}
	if api.Remaining != 0 {
		t.Errorf("api.Remaining = %d, want 0", api.Remaining)
	}
	if resolvers != nil {
		t.Errorf("expected nil resolvers, got %+v", resolvers)
	}
}

func TestParseRateLimits_Empty(t *testing.T) {
	now := time.Now()
	api, resolvers := parseRateLimits("", now)
	if api != nil || resolvers != nil {
		t.Errorf("expected nil for empty header, got api=%v resolvers=%v", api, resolvers)
	}
}

func TestAPILimitOK_FreshClient(t *testing.T) {
	c := &HFClient{}
	if !c.APILimitOK() {
		t.Error("fresh client should report APILimitOK=true")
	}
	if !c.ResolversLimitOK() {
		t.Error("fresh client should report ResolversLimitOK=true")
	}
}

func TestAPILimitOK_HasRemaining(t *testing.T) {
	c := &HFClient{}
	c.apiLimit = RateLimitBucket{Remaining: 100, ResetAt: time.Now().Add(5 * time.Minute)}
	if !c.APILimitOK() {
		t.Error("should be OK with remaining > 0")
	}
}

func TestAPILimitOK_Exhausted(t *testing.T) {
	c := &HFClient{}
	c.apiLimit = RateLimitBucket{Remaining: 0, ResetAt: time.Now().Add(5 * time.Minute)}
	if c.APILimitOK() {
		t.Error("should NOT be OK with remaining=0 and reset in future")
	}
}

func TestAPILimitOK_ResetPassed(t *testing.T) {
	c := &HFClient{}
	c.apiLimit = RateLimitBucket{Remaining: 0, ResetAt: time.Now().Add(-1 * time.Second)}
	if !c.APILimitOK() {
		t.Error("should be OK after reset time has passed")
	}
}

func TestNextResetTime_NoExhausted(t *testing.T) {
	c := &HFClient{}
	c.apiLimit = RateLimitBucket{Remaining: 100, ResetAt: time.Now().Add(5 * time.Minute)}
	c.resLimit = RateLimitBucket{Remaining: 200, ResetAt: time.Now().Add(5 * time.Minute)}

	reset := c.NextResetTime()
	if !reset.IsZero() {
		t.Errorf("expected zero time when no bucket exhausted, got %v", reset)
	}
}

func TestNextResetTime_OneExhausted(t *testing.T) {
	expected := time.Now().Add(3 * time.Minute)
	c := &HFClient{}
	c.apiLimit = RateLimitBucket{Remaining: 0, ResetAt: expected}
	c.resLimit = RateLimitBucket{Remaining: 100, ResetAt: time.Now().Add(5 * time.Minute)}

	reset := c.NextResetTime()
	if !reset.Equal(expected) {
		t.Errorf("NextResetTime = %v, want %v", reset, expected)
	}
}
