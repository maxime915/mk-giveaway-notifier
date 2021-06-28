package main

import (
	"fmt"
	"github.com/turnage/graw"
	"github.com/turnage/graw/reddit"
	"github.com/turnage/graw/streams"
	"time"
)

const (
	delay        = 2 * time.Second
	useChannel   = false
	agentFile    = "reddit_agentfile"
	subreddit    = "pics"
	subredditUrl = "/r/" + subreddit
)

type Feed struct {
	bot   reddit.Bot
	after string
	Post  chan *reddit.Post
	Errs  chan error
	Kill  chan bool
}

func NewFeed() (*Feed, error) {
	bot, err := reddit.NewBotFromAgentFile(agentFile, delay)
	if err != nil {
		return nil, err
	}

	feed := &Feed{
		bot:   bot,
		after: "",
		Post:  make(chan *reddit.Post, 110),
		Errs:  make(chan error, 1),
		Kill:  make(chan bool, 1),
	}

	go feed.run()
	return feed, nil
}

func (f *Feed) produce() {
	harvest, err := f.bot.Listing(subredditUrl, f.after)
	if err != nil {
		f.Errs <- err
		f.Kill <- true
		return
	}

	for _, post := range harvest.Posts {
		f.Post <- post
	}

	// update the reference point
	f.after = harvest.Posts[len(harvest.Posts)-1].Name
}

func (f *Feed) run() {
	for {
		select {
		case <-f.Kill:
			close(f.Post)
			close(f.Errs)
			return
		case <-time.After(delay):
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

func channel() error {
	feed, err := NewFeed()
	if err != nil {
		fmt.Println(err)
		return err
	}

	counter := 0
	dict := map[string]struct{}{}
	for post := range feed.Post {
		fmt.Printf("(%d) : [%s] posted\n", counter, post.Author)
		if _, ok := dict[post.ID]; ok {
			fmt.Println("duplicated value : ", post.Author)
		} else {
			dict[post.ID] = struct{}{}
		}
		counter++
	}

	for err := range feed.Errs {
		fmt.Println("Failed to fetch "+subredditUrl+": ", err.Error())
	}

	return nil
}

func useStream() {
	bot, err := reddit.NewBotFromAgentFile(agentFile, 0)
	if err != nil {
		fmt.Println("Failed to create bot handle: ", err)
		return
	}

	kill := make(chan bool, 1)
	errs := make(chan error, 1)
	post, err := streams.Subreddits(bot, kill, errs, "askreddit", "news", "pics", "funny")
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
				fmt.Println("duplicated value : ", p.Author)
			}
			track[p.ID] = p.Name
			fmt.Println("found ", len(track), " unique posts (so far)")
			if len(track) > 300 {
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
