package main

import (
	"fmt"
	"time"
)

// RateLimiter synchronization mechanism to avoid going over the limit of an API.
type RateLimiter interface {
	// Wait blocks to reserve a slot, returns with an error if the limiter is stopped
	Wait() error
	// TryWait returns true if a slot was reserved, false otherwise (non blocking)
	TryWait() bool
	// Stop prevents the reservation of new slot
	Stop()
}

type rateLimiter struct {
	size int
	slot chan struct{}
	kill chan struct{}
}

// NewRateLimiter creates a safe-to-copy RateLimiter, panic if perMinute < 1
func NewRateLimiter(perMinute int) RateLimiter {
	if perMinute < 1 {
		panic("invalid limit given to NewRateLimiter")
	}

	limiter := &rateLimiter{
		size: perMinute,
		slot: make(chan struct{}, 0), // unbuffered -> rendezvous, booking
		kill: make(chan struct{}, 1), // small buffer, enough for non-blocking
	}

	go limiter.run()
	return limiter
}

func (limiter *rateLimiter) run() {
	ticker := time.NewTicker(time.Minute)
	available := limiter.size

	for {
		if available == 0 {
			select {
			case <-limiter.kill:
				close(limiter.slot)
				ticker.Stop()
				return
			case <-ticker.C:
				available = limiter.size
			}
			continue
		}

		select {
		case <-limiter.kill:
			close(limiter.slot)
			ticker.Stop()
			return
		case <-ticker.C:
			available = limiter.size
		case limiter.slot <- struct{}{}:
			available -= 1
		}
	}
}

func (limiter *rateLimiter) Wait() error {
	_, ok := <-limiter.slot
	if !ok {
		return fmt.Errorf("limiter was closed")
	}
	return nil
}

func (limiter *rateLimiter) TryWait() bool {
	select {
	case _, ok := <-limiter.slot:
		return ok
	default:
		return false
	}
}

func (limiter *rateLimiter) Stop() {
	limiter.kill <- struct{}{}
}
