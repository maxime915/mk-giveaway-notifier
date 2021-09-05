package reddit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vartanbeno/go-reddit/v2/reddit"
)

var defaultBot *Bot = NewRedditBot()

func DefaultBot() *Bot {
	return defaultBot
}

type Post = reddit.Post

// Position stores the name and creation date of a reddit post
// it is used to locate the post in the feed later
type Position struct {
	FullID  string           `json:"name"`
	Created reddit.Timestamp `json:"created_utc"`
}

// Anchor is a slice of FeedPosition that follow each other
// it adds information in case one or more posts would be deleted.
// Order should be the same as the r/.../new page.
type Anchor = []Position

// Feed represent a list of subreddit to fetch (by /new) and a reference in
// the feed. Feed may be created via the Bot.NewFeed(...string) function. If
// you create Feed by hand, you may use Bot.Touch(*Feed) to set their reference
// to the newest posts.
type Feed struct {
	Anchor     Anchor `json:"anchor"`
	Subreddits string `json:"url"`
}

// NewRedditBot creates a reddit API handles without any loggin
func NewRedditBot() *Bot {
	// will not fail without argument
	client, _ := reddit.NewReadonlyClient()
	return &Bot{
		client,
		newRateLimiter(),
	}
}

// Bot wraps around github.com/vartanbeno/go-reddit/v2/reddit with a rate limiter
type Bot struct {
	client      *reddit.Client
	ratelimiter *ratelimiter
}

func (bot Bot) newPosts(subreddit, before, after string, limit int) ([]*reddit.Post, error) {
	bot.ratelimiter.Book()

	posts, resp, err := bot.client.Subreddit.NewPosts(context.Background(), subreddit, &reddit.ListOptions{
		After:  after,
		Before: before,
		Limit:  limit,
	})

	// set ratelimiter with newer information
	bot.ratelimiter.Update(resp.Rate)

	if err != nil {
		return nil, err
	}

	return posts, nil
}

func (bot Bot) getPost(id string) ([]*reddit.Post, error) {
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

	return posts, err
}

func (bot *Bot) checkPosition(postion Position) bool {
	posts, err := bot.getPost(postion.FullID)

	if err != nil {
		return false
	}

	return posts[0].Author != "[deleted]"
}

// NewFeed creates a new Feed referencing the newest posts of the subreddits
func (bot *Bot) NewFeed(subreddits ...string) (*Feed, error) {
	feed := &Feed{Subreddits: strings.Join(subreddits, "+")}
	err := bot.Touch(feed)
	if err != nil {
		return nil, err
	}
	return feed, nil
}

func (bot *Bot) Touch(feed *Feed) error {
	// 3 posts for the anchor is a safe measure
	posts, err := bot.newPosts(feed.Subreddits, "", "", 3)
	if err != nil {
		return err
	}

	// copy the value in the anchor
	feed.Anchor = make([]Position, len(posts))
	for i := range posts {
		feed.Anchor[i].FullID = posts[i].FullID
		feed.Anchor[i].Created = *posts[i].Created
	}

	return nil
}

// Peek fetches the reddit API
// Order: lowest index is most recent
// May return empty list without error
// feed.Anchor must have at least one element (see bot.Touch(*Feed))
func (bot *Bot) Peek(feed *Feed) ([]*reddit.Post, error) {
	// if no anchor available, impossible to have a reference in the feed
	if len(feed.Anchor) == 0 {
		return nil, fmt.Errorf("unable to crawl with empty ref")
	}

	// find the first valid saved reference
	// (references might be deleted between two calls to Update)
	validIndex := -1
	for i := range feed.Anchor {
		if bot.checkPosition(feed.Anchor[i]) {
			validIndex = i
			break
		}
	}

	// if none found, use time instead
	if validIndex == -1 {
		panic("crawl using time not supported yet") // FIXME
	}

	check := make(map[string]struct{}, len(feed.Anchor))
	for i := range feed.Anchor {
		check[feed.Anchor[i].FullID] = struct{}{}
	}

	before := feed.Anchor[validIndex].FullID
	result := make(map[int][]*reddit.Post)
	totalLength := 0

	for {
		posts, err := bot.newPosts(feed.Subreddits, before, "", 100)

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

// Update fetches the reddit API (see bot.Peek(*Feed)) and set the state of the feed
func (bot *Bot) Update(feed *Feed) ([]*reddit.Post, error) {

	// get posts
	posts, err := bot.Peek(feed)
	if err != nil {
		return nil, err
	}

	// build anchor from fetched posts
	anchorSize := cap(feed.Anchor)
	if anchorSize > len(posts) {
		anchorSize = len(posts)
	}
	newAnchor := make([]Position, anchorSize)
	for i := 0; i < anchorSize; i++ {
		newAnchor[i].FullID = posts[i].FullID
		newAnchor[i].Created = *posts[i].Created
	}

	// make sure the anchor size does not decrease
	delta := cap(feed.Anchor) - len(posts)
	if delta > 0 {
		newAnchor = append(newAnchor, feed.Anchor[:delta]...)
	}

	feed.Anchor = newAnchor

	return posts, nil
}

// Poll fetches the red dit API up to a date (ignoring any state anchor)
func (bot *Feed) Poll(feed Feed, duration time.Duration) ([]*reddit.Post, error) {
	panic("implement me") // FIXME
}
