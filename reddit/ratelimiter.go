package reddit

import (
	"sync"
	"time"

	"github.com/vartanbeno/go-reddit/v2/reddit"
)

const minRemaining = 2

// rate limiter for the reddit API
type ratelimiter struct {
	mutex *sync.Mutex
	rate  reddit.Rate
}

func newRateLimiter() *ratelimiter {
	// avoid sleeping on invalid datetime for the first call
	return &ratelimiter{
		mutex: &sync.Mutex{},
		rate:  reddit.Rate{Remaining: minRemaining + 1},
	}
}

// reserve a slot
func (rl *ratelimiter) Book() {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	if rl.rate.Remaining < minRemaining {
		time.Sleep(time.Until(rl.rate.Reset))
		rl.rate.Remaining = 300
	}

	rl.rate.Remaining -= 1
}

// update with more up-to-date info
func (rl *ratelimiter) Update(rate reddit.Rate) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	rl.rate = rate
}
