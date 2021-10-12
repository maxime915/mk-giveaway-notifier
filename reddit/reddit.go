// reddit handles communication with the Reddit API.
// To obtain a bot, you can call DefaultBot() which
// returns a bot without any login information.
// You can then create a new Feed via the Bot.NewFeed(...string)
// method for a list of subreddit. This method requires an internet
// connection to fetch reddit's API and setup then Anchor of the Feed.
// Feed can be Marshal'ed/Unmarshal'ed although it is not necessary to do
// it manually : the telegram API does it.
package reddit

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/vartanbeno/go-reddit/v2/reddit"
)

var defaultBot *Bot = NewRedditBot()

// DefaultBot returns a bot without any login information.
// The rate of this bot will be limited to 300 requests / 10min per
// Reddit's API.
// The pointer returned by this function does not change.
func DefaultBot() *Bot {
	return defaultBot
}

// Post represent a reddit post with Title, Author, etc
type Post = reddit.Post

// Bot wraps around github.com/vartanbeno/go-reddit/v2/reddit with a rate limiter
type Bot struct {
	client      *reddit.Client
	ratelimiter *ratelimiter
}

// NewRedditBot creates a reddit API handles without any login information.
// The rate of this bot will be limited to 300 requests / 10min per
// Reddit's API.
func NewRedditBot() *Bot {
	// will not fail without argument
	client, _ := reddit.NewReadonlyClient()
	return &Bot{
		client,
		newRateLimiter(),
	}
}

// newPosts fetches new posts using the rate limiter
func (bot Bot) newPosts(subreddit, before, after string, limit int) ([]*reddit.Post, error) {
	bot.ratelimiter.Book()

	posts, resp, err := bot.client.Subreddit.NewPosts(context.Background(), subreddit, &reddit.ListOptions{
		After:  after,
		Before: before,
		Limit:  limit,
	})

	if err != nil {
		return nil, err
	}

	// set ratelimiter with newer information
	bot.ratelimiter.Update(resp.Rate)

	return posts, nil
}

// getPost fetches the information of 1 post
func (bot Bot) getPost(id string) (*reddit.Post, error) {
	bot.ratelimiter.Book()

	posts, resp, err := bot.client.Listings.GetPosts(context.Background(), id)

	if err != nil {
		return nil, err
	}

	// set ratelimiter with newer information
	bot.ratelimiter.Update(resp.Rate)

	if len(posts) != 1 {
		return nil, fmt.Errorf("expected 1 post for getPost(%s)", id)
	}

	return posts[0], err
}

// checkPosition makes sure the position points to a valid, non-deleted post
func (bot *Bot) checkPosition(postion Position) bool {
	post, err := bot.getPost(postion.FullID)

	if err != nil {
		return false
	}

	return post.Author != "[deleted]"
}

// Touch sets the anchor of the feed to the most recent posts of the sub
func (bot *Bot) Touch(feed *Feed) ([]*reddit.Post, error) {
	limit := 5
	return bot.fetchAndUpdateAnchor(feed, limit, func() ([]*reddit.Post, error) {
		return bot.newPosts(feed.Subreddits, "", "", limit)
	})
}

func (bot *Bot) peekBefore(subreddits, before string) ([]*reddit.Post, error) {
	result := make(map[int][]*reddit.Post)
	totalLength := 0

	for {
		posts, err := bot.newPosts(subreddits, before, "", 100)

		if err != nil {
			return nil, err
		}

		// empty : "Before" is supposedly valid so we reached the top, stop here
		if len(posts) == 0 {
			break
		}

		// add fetched results to the list
		result[len(result)] = posts
		totalLength += len(posts)

		// update reference
		before = posts[0].FullID
	}

	// join all slices
	joined := make([]*reddit.Post, totalLength)
	low := 0
	for k := len(result) - 1; k >= 0; k-- {
		low += copy(joined[low:], result[k])
	}

	return joined, nil
}

// Peek fetches the reddit API to retrieve all posts newer than the feed's anchor.
// Feed.Anchor must have at least one element. See bot.Touch(*Feed) .
// The returned posts are returned in newest-first order. This function may return
// an empty list without error.
func (bot *Bot) Peek(feed *Feed) ([]*reddit.Post, error) {
	var results []*reddit.Post
	var err error

	// if no anchor available, impossible to have a reference in the feed
	if len(feed.Anchor) == 0 {
		return nil, fmt.Errorf("unable to crawl with empty ref")
	}

	// try all anchor points, newest first
	for _, position := range feed.Anchor {
		if !bot.checkPosition(position) {
			log.Printf("invalid index for position %+v\n", position)
			continue
		}

		results, err = bot.peekBefore(feed.Subreddits, position.FullID)

		if err != nil {
			return nil, err
		}

		if len(results) > 0 {
			break
		}
	}

	// unable to fetch from the anchor, crawl to saved date instead
	if len(results) == 0 {
		return bot.crawl(feed)
	}

	return results, nil
}

// Update fetches the reddit API to retrieve all posts newer than the feed's anchor,
// and then updates the feed's anchor to the newest fetched posts.
// Feed.Anchor must have at least one element. See bot.Touch(*Feed) .
// The returned posts are returned in newest-first order. This function may return
// an empty list without error.
// Feed.Anchor is not written to in case of any error.
func (bot *Bot) Update(feed *Feed) ([]*reddit.Post, error) {
	return bot.UpdateForAnchorSize(feed, len(feed.Anchor))
}

func (bot *Bot) UpdateForAnchorSize(feed *Feed, anchorSize int) ([]*reddit.Post, error) {
	return bot.fetchAndUpdateAnchor(feed, anchorSize, func() ([]*reddit.Post, error) {
		return bot.Peek(feed)
	})
}

func (bot *Bot) fetchAndUpdateAnchor(feed *Feed, anchorSize int, fetch func() ([]*reddit.Post, error)) ([]*reddit.Post, error) {
	// get posts
	posts, err := fetch()
	if err != nil {
		return nil, err
	}

	// make sure there are enough post to build the anchor
	newEntryCount := anchorSize
	if newEntryCount > len(posts) {
		// old anchor may be used if needed
		if newEntryCount > len(feed.Anchor)+len(posts) {
			return nil, fmt.Errorf("not enough post to update the anchor size")
		}
		newEntryCount = len(posts)
	}

	// build anchor from fetched posts
	newAnchor := make([]Position, newEntryCount, anchorSize)

	// add newest posts first
	for i := 0; i < newEntryCount; i++ {
		newAnchor[i].FullID = posts[i].FullID
		newAnchor[i].Created = *posts[i].Created
	}

	// make sure the anchor size does not decrease
	delta := anchorSize - newEntryCount
	if delta > 0 {
		newAnchor = append(newAnchor, feed.Anchor[:delta]...)
	}

	feed.Anchor = newAnchor
	return posts, nil
}

func (bot *Bot) crawl(feed *Feed) ([]*reddit.Post, error) {
	// if no anchor available, impossible to have a reference in the feed
	if len(feed.Anchor) == 0 {
		return nil, fmt.Errorf("unable to crawl with empty ref")
	}

	// minimum time of publication as a reference (-> oldest posts)
	target := minOfAnchor(feed.Anchor)

	return bot.crawlUntil(target, feed.Subreddits)
}

func (bot *Bot) crawlUntil(target time.Time, subreddit string) ([]*reddit.Post, error) {
	result := make(map[int][]*reddit.Post)
	totalLength := 0

	notFound := true
	after := ""
	for notFound {
		posts, err := bot.newPosts(subreddit, "", after, 100)

		if err != nil {
			return nil, err
		}

		// should have fetched 100 posts... maybe there was an error?
		if len(posts) < 5 {
			return nil, fmt.Errorf("unable to fetch 100 posts")
		}

		rm := newRollingMedian(posts[:5])
		if rm.cachedValue.Before(target) {
			// break -> but save post firsts
			notFound = false
			posts = posts[:5]
			goto save
		}

		for k := 5; k < len(posts); k++ {
			if rm.add(posts[k]).Before(target) {
				// break main loop -> but save post firsts
				notFound = false
				posts = posts[:k]
				goto save
			}
		}

	save:
		// add fetched results to the list
		result[len(result)] = posts
		totalLength += len(posts)

		// update reference
		after = posts[len(posts)-1].FullID
	}

	// join all slices -> newest post first
	joined := make([]*reddit.Post, totalLength)
	low := 0
	for k := range result {
		low += copy(joined[low:], result[k])
	}

	return joined, nil
}

// Poll fetches the red dit API up to a date (ignoring any state anchor)
func (bot *Bot) Poll(feed *Feed, duration time.Duration) ([]*reddit.Post, error) {
	return bot.crawlUntil(time.Now().Add(-duration), feed.Subreddits)
}
