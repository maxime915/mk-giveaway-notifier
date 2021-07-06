package main

import (
	"fmt"
	"time"
)

type RateLimiter interface {
	// block to reserve a slot, returns with an error if the limiter is stopped
	Wait() error
	// stop the limiter : no new slot will be created, call to Wait will succeed
	// until they would normally wait, in this case they will return an error
	Stop()
}

type rateLimiter struct {
	slot chan struct{}
	done chan struct{}
}

// returns a safe-to-copy RateLimiter, panic if perMinute < 1
func NewRateLimiter(perMinute int) RateLimiter {
	if perMinute < 1 {
		panic("invalid limit given to NewRateLimiter")
	}

	limiter := &rateLimiter{
		slot: make(chan struct{}, perMinute),
		done: make(chan struct{}, 1),
	}
	go limiter.run()
	limiter.fillUp()
	return limiter
}

func (limiter *rateLimiter) run() {
	ticker := time.NewTicker(time.Minute)
	for {
		select {
		case <-limiter.done:
			ticker.Stop()
			close(limiter.slot)
			return
		case <-ticker.C:
			limiter.fillUp()
		}
	}
}

func (limiter *rateLimiter) fillUp() {
	delta := cap(limiter.slot) - len(limiter.slot)
	for i := 0; i < delta; i++ {
		limiter.slot <- struct{}{}
	}
}

func (limiter *rateLimiter) Wait() error {
	_, ok := <-limiter.slot
	if !ok {
		return fmt.Errorf("limiter was closed")
	}
	return nil
}

func (limier *rateLimiter) Stop() {
	limier.done <- struct{}{}
}
