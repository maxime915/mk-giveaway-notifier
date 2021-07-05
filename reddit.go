package main

import (
	"fmt"
	"time"

	"github.com/turnage/graw/reddit"
)

const (
	delay        = 2 * time.Second
	agentFile    = "reddit_agentfile"
	subreddit    = "askreddit"
	subredditUrl = "/r/" + subreddit + "/new"
)

type Feed struct {
	bot   reddit.Bot
	After string
	Delay time.Duration
	Post  chan *reddit.Post
	Errs  chan error
	Kill  chan bool
}

func NewFeed() (*Feed, error) {
	return NewFeedAfter("")
}

func NewFeedAfter(postName string) (*Feed, error) {
	bot, err := reddit.NewBotFromAgentFile(agentFile, delay)
	if err != nil {
		return nil, err
	}

	feed := &Feed{
		bot:   bot,
		After: postName,
		Delay: delay,
		Post:  make(chan *reddit.Post, 110),
		Errs:  make(chan error, 1),
		Kill:  make(chan bool, 1),
	}

	go feed.run()
	return feed, nil
}

func (f *Feed) produce() {
	harvest, err := f.bot.Listing(subredditUrl, f.After)
	if err != nil {
		f.Errs <- err
		f.Kill <- true
		return
	}

	// send them in chronological order
	for i := len(harvest.Posts); i >= 0; i-- {
		f.Post <- harvest.Posts[i]
	}

	// update reference if possible
	if len(harvest.Posts) != 0 {
		f.After = harvest.Posts[0].Name
	}
}

func (f *Feed) run() {
	for {
		select {
		case <-f.Kill:
			close(f.Post)
			close(f.Errs)
			return
		case <-time.After(f.Delay):
			f.produce()
		}
	}
}

// channel polls reddit API for all post submitted after a given post, delay is
// not fixed.
func channel() error {
	feed, err := NewFeedAfter("")
	if err != nil {
		fmt.Println(err)
		return err
	}

	counter := 0
	dict := map[string]struct{}{}
	for post := range feed.Post {
		if _, ok := dict[post.ID]; ok {
			fmt.Println("duplicated value : ", post.Title)
		} else {
			// created := time.Unix(int64(post.CreatedUTC), 0)
			// fmt.Printf("(%3d) : %v, %s\n", counter, created.Format("Mon Jan 2 15:04:05 -0700 MST 2006"), post.Title)
			dict[post.ID] = struct{}{}
		}
		counter++
	}

	for err := range feed.Errs {
		fmt.Println("Failed to fetch "+subredditUrl+": ", err.Error())
	}

	return nil
}
