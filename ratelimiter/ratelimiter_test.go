package ratelimiter

import (
	"testing"
	"time"
)

func TestLimiterOK(t *testing.T) {
	limiter := NewRateLimiter(3, time.Minute)
	timer := time.After(3 * time.Second)

	for i := 0; i < 3; i++ {
		if !limiter.Book() {
			t.Errorf("unable to book!")
		}
	}

	select {
	case <-timer:
		t.Errorf("booking took too long")
	default:
	}
}

func TestLimiterStopFull(t *testing.T) {
	limiter := NewRateLimiter(3, time.Minute)

	for i := 0; i < 3; i++ {
		if !limiter.Book() {
			t.Error("unable to book!")
		}
	}

	limiter.Stop()

	if limiter.Book() {
		t.Error("should not have been able to book a closed limiter")
	}
}

func TestLimiterStopNotFull(t *testing.T) {
	limiter := NewRateLimiter(3, time.Minute)

	for i := 0; i < 2; i++ {
		if !limiter.Book() {
			t.Error("unable to book!")
		}
	}

	limiter.Stop()

	if limiter.Book() {
		t.Error("should not have been able to book a closed limiter")
	}
}
