package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/turnage/graw/reddit"
)

/*FIXME
there seems to be an issue where the saved name is no longer valid after an
hibernation, hard to reproduce.
could be:
	- post deleted/removed
	- some way that fullname is change in the reddit API
	- something else ?
*/

/*FIXME
if we are more than 200 posts behind
e.g.
0, 1, 2, ..., 100, 101, ..., 200, 201, ... Feed.After, ...
post are not going to be sent in the correct order...
*/

/*FIXME
what if the Feed.After was removed ?
we would not get any more post...
check via CreatedUTC seems a better option*/

const (
	minDelay     = 2 * time.Second
	delay        = 2 * time.Second
	agentFile    = "reddit_agentfile"
	subreddit    = "askreddit"
	subredditUrl = "/r/" + subreddit + "/new"
)

// TODO save mor data and embed RedditData into the Feed
type RedditData struct {
	Position string `json:"position"`
}

type Feed struct {
	bot   reddit.Bot
	Url   string
	After string
	Delay time.Duration
	Post  chan *reddit.Post
	Errs  chan error
	Kill  chan bool
}

func NewFeed() (*Feed, error) {
	return NewFeedFromData(RedditData{""})
}

func NewFeedFromData(data RedditData) (*Feed, error) {
	bot, err := reddit.NewBotFromAgentFile(agentFile, delay)
	if err != nil {
		return nil, err
	}

	feed := &Feed{
		bot:   bot,
		Url:   subredditUrl,
		After: data.Position,
		Delay: delay,
		Post:  make(chan *reddit.Post, 110),
		Errs:  make(chan error, 1),
		Kill:  make(chan bool, 1),
	}

	go feed.run()
	return feed, nil
}

func (f *Feed) Update(data *RedditData) {
	data.Position = f.After
}

func (f *Feed) produce() {
	harvest, err := f.bot.Listing(f.Url, f.After)
	if err != nil {
		f.Errs <- err
		f.Kill <- true
		return
	}

	// send them in chronological order
	for i := len(harvest.Posts) - 1; i >= 0; i-- {
		f.Post <- harvest.Posts[i]
	}

	// update reference if possible
	if len(harvest.Posts) != 0 {
		f.After = harvest.Posts[0].Name
	}
}

func (f *Feed) run() {
	f.produce()
	ticker := time.NewTicker(delay)

	for {
		select {
		case <-f.Kill:
			close(f.Post)
			close(f.Errs)
			ticker.Stop()
			return
		case <-ticker.C:
			f.produce()
		}
	}
}

func (feed *Feed) Listen(postCallBack func(*reddit.Post)) error {
	feed, err := NewFeed()
	if err != nil {
		fmt.Println(err)
		return err
	}

	for post := range feed.Post {
		postCallBack(post)
	}

	// there will be at most one error because we exit right after
	for err := range feed.Errs {
		return err
	}

	return nil
}

/*crawler approach
- store most-recent UTC in JSON
- start by crawling back to the saved point (fetching all post up to that)
- then store the most-recent UTC in the structure
- repeat =)*/

// TODO use a RateLimiter

// If if fails up to the very first one, should we re-try ?
func (feed *Feed) crawlBackTo(date uint64) ([]*reddit.Post, error) {
	// fetch at least once
	harvest, err := feed.bot.Listing(feed.Url, "")
	if err != nil {
		return nil, err
	}

	ticker := time.NewTicker(minDelay)
	result := harvest.Posts

	for result[len(result)-1].CreatedUTC < date {
		<-ticker.C
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
		return result[i].CreatedUTC >= date
	})

	return result[:bound], nil
}

// check for giveaway title (could be improved)
func IsGiveAway(postTitle string) bool {
	title := strings.ToLower(postTitle)
	return strings.Contains(title, "giveaway")
}

// the Feed polls reddit API for all post submitted after a given post, delay is
// not fixed.
func example() {
	feed, err := NewFeed()
	if err != nil {
		panic(err)
	}

	counter := 0
	dict := map[string]struct{}{}
	for post := range feed.Post {
		if _, ok := dict[post.ID]; ok {
			fmt.Println("duplicated value : ", post.Title)
		} else {
			created := time.Unix(int64(post.CreatedUTC), 0)
			fmt.Printf("(%3d) : %v, %s\n", counter, created.Format("Mon Jan 2 15:04:05 -0700 MST 2006"), post.Title)
			dict[post.ID] = struct{}{}
		}
		counter++
	}

	for err := range feed.Errs {
		fmt.Println("Failed to fetch "+subredditUrl+": ", err.Error())
	}
}
