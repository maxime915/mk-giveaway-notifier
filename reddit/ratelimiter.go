package reddit

import (
	"sync"
	"time"

	"github.com/vartanbeno/go-reddit/v2/reddit"
)

const minRemaining = 2

// rate limiter for the reddit API
// This ratelimiter works with best effort : there is no way to know if another
// client is using the same identifiers so the actual number of remaining calls
// may be lower than estimated.
type ratelimiter struct {
	mutex *sync.Mutex
	rate  reddit.Rate
}

// newRateLimiter return a new, valid ratelimiter
func newRateLimiter() *ratelimiter {
	// avoid sleeping on invalid datetime for the first call
	return &ratelimiter{
		mutex: &sync.Mutex{},
		rate:  reddit.Rate{Remaining: minRemaining + 1},
	}
}

// Book reserves a slot, waiting if necessary to avoid going
// over the limit of the API.
func (rl *ratelimiter) Book() {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	if rl.rate.Remaining < minRemaining {
		time.Sleep(time.Until(rl.rate.Reset))
		rl.rate.Remaining = 300
	}

	rl.rate.Remaining -= 1
}

// Update sets the information of the ratelimiter to more up to date information
func (rl *ratelimiter) Update(rate reddit.Rate) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	rl.rate = rate
}
