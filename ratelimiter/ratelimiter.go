package ratelimiter

import (
	"time"
)

// RateLimiter synchronization mechanism to avoid going over the limit of an API.
// Concurrent call to Book return in an unspecified order.
type RateLimiter interface {
	// Book blocks to reserve a slot, returns false if the RateLimiter was stopped
	Book() bool
	// Stop prevents the reservation of new slot
	Stop()
}

type rateLimiter struct {
	interval  time.Duration // should stay const
	frequency int           // should stay const
	slot      chan chan struct{}
	kill      chan struct{}
	done      chan struct{}
}

// NewRateLimiter creates a safe-to-copy RateLimiter, panic if frequency or interval < 1
func NewRateLimiter(frequency int, interval time.Duration) RateLimiter {
	if frequency < 1 {
		panic("invalid frequency")
	}
	if interval < 1 {
		panic("invalid interval")
	}

	limiter := &rateLimiter{
		interval:  interval,
		frequency: frequency,
		slot:      make(chan chan struct{}, frequency),
		kill:      make(chan struct{}, 1),
		done:      make(chan struct{}, 0),
	}

	go limiter.run()
	return limiter
}

func (limiter *rateLimiter) run() {
	ticker := time.NewTicker(limiter.interval)
	available := limiter.frequency

	pending := make(chan chan struct{}, limiter.frequency)

	for {
		select {
		case <-limiter.kill:
			close(limiter.done)
			ticker.Stop()
			return
		case <-ticker.C:
			available = limiter.frequency
			for ; available > 0 && len(pending) > 0; available-- {
				channel := <-pending
				channel <- struct{}{}
			}
		case channel := <-limiter.slot:
			if available == 0 {
				pending <- channel
			} else {
				channel <- struct{}{}
				available -= 1
			}
		}
	}
}

func (limiter *rateLimiter) Book() bool {
	channel := make(chan struct{}, 1)
	limiter.slot <- channel

	select {
	case <-channel:
		return true
	case <-limiter.done:
		return false
	}
}

func (limiter *rateLimiter) Stop() {
	limiter.kill <- struct{}{}
}
