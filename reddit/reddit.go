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
	"strings"
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

// Position stores the name and creation date of a reddit post
// it is used to locate the post in the feed later
type Position struct {
	FullID  string           `json:"name"`
	Created reddit.Timestamp `json:"created_utc"`
}

// Anchor is a slice of FeedPosition that follow each other
// it adds information in case one or more posts would be deleted.
// Lower index means newer position.
type Anchor = []Position

// Feed represent a list of subreddit to fetch (by /new) and a reference in
// the feed. Feed may be created via the Bot.NewFeed(...string) function. If
// you create Feed by hand, you may use Bot.Touch(*Feed) to set their reference
// to the newest posts.
type Feed struct {
	Anchor     Anchor `json:"anchor"`
	Subreddits string `json:"url"`
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

// Bot wraps around github.com/vartanbeno/go-reddit/v2/reddit with a rate limiter
type Bot struct {
	client      *reddit.Client
	ratelimiter *ratelimiter
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

// NewFeed creates a new Feed of a list of subreddits
// The Feed is then Touch'ed automatically by the Bot to create
// a valid anchor. Calling Bot.Peek(*Feed) or Bot.Update(*Feed) right
// after creation might return an empty list if the sub isn't very active.
func (bot *Bot) NewFeed(subreddits ...string) (*Feed, error) {
	if len(subreddits) < 1 {
		return nil, fmt.Errorf("at least 1 subreddit is required to create a Feed")
	}

	feed := &Feed{Subreddits: strings.Join(subreddits, "+")}

	err := bot.Touch(feed)
	if err != nil {
		return nil, err
	}

	return feed, nil
}

// Touch sets the anchor of the feed to the most recents posts of the sub
func (bot *Bot) Touch(feed *Feed) error {
	size := 3
	if cap(feed.Anchor) > size {
		size = cap(feed.Anchor)
	}

	// `size` posts for the anchor is a safe measure
	posts, err := bot.newPosts(feed.Subreddits, "", "", size)
	if err != nil {
		return err
	}

	if len(posts) < 1 {
		return fmt.Errorf("not enough posts found in the feed, aborting")
	}

	// if len(posts) < size
	// failing is not helping

	// copy the value in the anchor
	feed.Anchor = make([]Position, len(posts))
	for i := range posts {
		feed.Anchor[i].FullID = posts[i].FullID
		feed.Anchor[i].Created = *posts[i].Created
	}

	return nil
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

	if len(results) == 0 {
		return nil, fmt.Errorf("no post found by ID fetching & time based crawl is not supported yet")
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
