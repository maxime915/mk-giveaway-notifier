package ratelimiter

import (
	"time"
)

// RateLimiter synchronization mechanism to limit access to a resource over time.
// Only a certain amount of call to Book will be made in a given amount of time,
// any additional attemp will block to avoid overrusing the resource and will
// only resume after the time has passed.
type RateLimiter interface {
	// Book blocks to reserve a call, returns false if the RateLimiter was stopped.
	// Concurrent calls to Book return in an unspecified order.
	Book() bool
	// Stop prevents the reservation of new call.
	Stop()
}

type rateLimiter struct {
	order chan struct{}
	grant chan struct{}

	kill chan struct{}
	done chan struct{} // read-only-when-closed channel
}

// NewRateLimiter creates a safe-to-copy RateLimiter,
// panic if count or interval < 1
func NewRateLimiter(count int, interval time.Duration) RateLimiter {
	if count < 1 {
		panic("invalid count")
	}
	if interval < 1 {
		panic("invalid interval")
	}

	limiter := &rateLimiter{
		order: make(chan struct{}, count),
		grant: make(chan struct{}, count),
		kill:  make(chan struct{}, 1),
		done:  make(chan struct{}, 0),
	}

	go limiter.run(count, interval)
	return limiter
}

func (limiter *rateLimiter) run(count int, interval time.Duration) {
	ticker := time.NewTicker(interval)

	available := count
	pending := 0

	for {
		select {
		case <-limiter.kill:
			close(limiter.done)
			ticker.Stop()
			return
		case <-ticker.C:
			available = count
			for available > 0 && pending > 0 {
				limiter.grant <- struct{}{}
				available--
				pending--
			}
		case <-limiter.order:
			if available != 0 {
				limiter.grant <- struct{}{}
				available--
			} else {
				pending++
			}
		}
	}
}

func (limiter *rateLimiter) Book() bool {
	limiter.order <- struct{}{}
	select {
	case <-limiter.grant:
		return true
	case <-limiter.done:
		return false
	}
}

func (limiter *rateLimiter) Stop() {
	limiter.kill <- struct{}{}
}
