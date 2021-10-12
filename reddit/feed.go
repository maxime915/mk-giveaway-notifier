package reddit

import (
	"fmt"
	"strings"

	"github.com/vartanbeno/go-reddit/v2/reddit"
)

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
	// Anchor is a list of position, with newest posts first
	Anchor     Anchor `json:"anchor"`
	Subreddits string `json:"url"`
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
