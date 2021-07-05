package main

import (
	"fmt"
	"time"

	"github.com/turnage/graw"
	"github.com/turnage/graw/reddit"
	"github.com/turnage/graw/streams"
)

const (
	delay        = 2 * time.Second
	useChannel   = true
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

func raw() {
	bot, err := reddit.NewBotFromAgentFile(agentFile, delay)
	if err != nil {
		fmt.Println("Failed to create bot handle: ", err)
		return
	}

	harvest, err := bot.Listing(subredditUrl, "")
	if err != nil {
		fmt.Println("Failed to fetch "+subredditUrl+": ", err)
		return
	}

	fmt.Println("fetched ", len(harvest.Posts), " posts")
}

type reminderBot struct {
	bot   reddit.Bot
	track map[string]string
}

func (r *reminderBot) Post(p *reddit.Post) error {
	if _, ok := r.track[p.ID]; ok {
		fmt.Println("duplicate post : ", p.ID)
	}
	r.track[p.ID] = p.Name
	fmt.Println("found ", len(r.track), " unique posts")
	return nil
}

func other() {
	bot, err := reddit.NewBotFromAgentFile(agentFile, delay)
	if err != nil {
		fmt.Println("Failed to create bot handle: ", err)
		return
	}

	cfg := graw.Config{Subreddits: []string{"golang"}}
	handler := &reminderBot{bot: bot, track: map[string]string{}}

	_, wait, err := graw.Run(handler, bot, cfg)

	if err != nil {
		fmt.Println("Failed to start graw run: ", err)
		return
	}

	fmt.Println("graw run failed: ", wait())
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

// useStream hooks up to reddit and only notify on new post (submitted after
// the hooks) & no delay configurable
func useStream() {
	bot, err := reddit.NewBotFromAgentFile(agentFile, 0)
	if err != nil {
		fmt.Println("Failed to create bot handle: ", err)
		return
	}

	kill := make(chan bool, 1)
	errs := make(chan error, 1)
	post, err := streams.Subreddits(bot, kill, errs, subreddit)
	if err != nil {
		fmt.Println("Failed to create stream: ", err)
		return
	}

	track := map[string]string{}

	for {
		select {
		case p := <-post:
			if p == nil {
				fmt.Println("got nil post...")
				break
			}
			fmt.Println(p.Name)
			if _, ok := track[p.ID]; ok {
				fmt.Println("duplicated post : ", p.Title)
			} else {
				fmt.Println("found post with title ", p.Title)
			}
			track[p.ID] = p.Name
			if len(track) > 10 {
				kill <- true
				return
			}
		case e := <-errs:
			fmt.Println(e)
		case <-time.After(2 * time.Minute):
			kill <- true
			return
		}
	}
}
