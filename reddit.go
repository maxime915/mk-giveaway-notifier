package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/maxime915/mk-giveaway-notifier/ratelimiter"

	"github.com/turnage/graw/reddit"
)

const (
	delay     = 2 * time.Second
	agentFile = "reddit_agentfile"
)

// RedditData stores persistent data
type RedditData struct {
	Subreddit   string        `json:"subreddit"`
	PositionUTC uint64        `json:"position"`
	Delay       time.Duration `json:"delay"`
}

// Feed offers a way to receive new posts
type Feed struct {
	RedditData
	bot  reddit.Bot
	Url  string
	Post chan *reddit.Post
	Errs chan error
	Kill chan bool
	rate ratelimiter.RateLimiter
	mut  *sync.Mutex
}

type Post = reddit.Post

func NewFeedFromData(data RedditData) (*Feed, error) {
	bot, err := reddit.NewBotFromAgentFile(agentFile, data.Delay)
	if err != nil {
		return nil, err
	}

	feed := &Feed{
		RedditData: data,
		bot:        bot,
		Url:        "/r/" + data.Subreddit + "/new",
		Post:       make(chan *reddit.Post, 110),
		Errs:       make(chan error, 1),
		Kill:       make(chan bool, 1),
		rate:       ratelimiter.NewRateLimiter(60, time.Minute),
		mut:        &sync.Mutex{},
	}

	go feed.run()
	return feed, nil
}

func (feed *Feed) Update(data *RedditData) {
	*data = feed.RedditData
}

func (feed *Feed) abort(err error) {
	feed.Errs <- err
	feed.Kill <- true
}

func (feed *Feed) fetchNewest() ([]*reddit.Post, error) {
	if !feed.rate.Book() {
		return nil, fmt.Errorf("could not book an slot")
	}

	harvest, err := feed.bot.Listing(feed.Url, "")
	if err != nil {
		return nil, err
	}

	return harvest.Posts, nil
}

/*crawler approach
- store most-recent UTC in JSON
- start by crawling back to the saved point (fetching all post up to that)
- then store the most-recent UTC in the structure
- repeat =)*/

// If if fails up to the very first one, should we re-try ?
func (feed *Feed) crawlBackTo(date uint64) ([]*reddit.Post, error) {
	// NB: result[...].CreatedUTC is not necessarily sorted

	// fetch at least once
	result, err := feed.fetchNewest()
	if err != nil {
		return nil, err
	}

	for result[len(result)-1].CreatedUTC > date {

		if !feed.rate.Book() {
			return nil, fmt.Errorf("could not book an slot")
		}
		harvest, err := feed.bot.ListingWithParams(feed.Url, map[string]string{
			"after": result[len(result)-1].Name,
		})
		if err != nil {
			return nil, err
		}

		// if the last loaded post was deleted reddit returns an empty list
		// -> drop the deleted post and re-try
		if len(harvest.Posts) == 0 {
			if len(result) == 0 {
				return nil, fmt.Errorf("could not load any post")
			}
			result = result[:len(result)-1]
			continue
		}

		result = append(result, harvest.Posts...)
	}

	bound := sort.Search(len(result), func(i int) bool {
		return result[i].CreatedUTC < date
	})

	return result[:bound], nil
}

func (feed *Feed) produce() {
	var result []*Post
	var err error

	if feed.PositionUTC != 0 {
		result, err = feed.crawlBackTo(feed.PositionUTC)
	} else {
		result, err = feed.fetchNewest()
	}

	if err != nil {
		feed.abort(err)
		return
	}

	for _, post := range result {
		feed.Post <- post
	}

	feed.PositionUTC = result[0].CreatedUTC
}

func (feed *Feed) run() {
	feed.produce()
	ticker := time.NewTicker(feed.Delay)

	for {
		select {
		case <-feed.Kill:
			close(feed.Post)
			close(feed.Errs)
			ticker.Stop()
			return
		case <-ticker.C:
			feed.produce()
		}
	}
}

func (feed *Feed) Listen(postCallBack func(*reddit.Post)) error {
	for post := range feed.Post {
		postCallBack(post)
	}

	// there will be at most one error because we exit right after
	for err := range feed.Errs {
		return err
	}

	return nil
}

// IsGiveAway checks for giveaway title (could be improved)
func IsGiveAway(postTitle string) bool {
	title := strings.ToLower(postTitle)
	return strings.Contains(title, "giveaway")
}
