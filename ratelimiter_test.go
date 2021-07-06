package main

import (
	"testing"
	"time"
)

func TestLimiterOK(t *testing.T) {
	limiter := NewRateLimiter(3)

	chrono := time.After(3 * time.Second)

	for i := 0; i < 3; i++ {
		err := limiter.Wait()
		if err != nil {
			t.Errorf("RateLimiter.Wait() returned %v", err)
		}
	}

	select {
	case <-chrono:
		t.Errorf("the 3 Wait() took more than 3 seconds")
	default:
	}
}

func TestLimiterStopFull(t *testing.T) {
	limiter := NewRateLimiter(3)

	for i := 0; i < 3; i++ {
		err := limiter.Wait()
		if err != nil {
			t.Errorf("RateLimiter.Wait() returned %v", err)
		}
	}

	limiter.Stop()

	err := limiter.Wait()
	if err == nil {
		t.Errorf("RateLimiter.Wait() should have returned an error")
	}
}

func TestLimiterStopNotFull(t *testing.T) {
	limiter := NewRateLimiter(3)

	for i := 0; i < 2; i++ {
		err := limiter.Wait()
		if err != nil {
			t.Errorf("RateLimiter.Wait() returned %v", err)
		}
	}

	limiter.Stop()

	err := limiter.Wait()
	if err != nil {
		t.Errorf("RateLimiter.Wait() returned %v", err)
	}

	err = limiter.Wait()
	if err == nil {
		t.Errorf("RateLimiter.Wait() should have returned an error")
	}
}
