// telegram handles the reception/reply of messages.
// You can create a new bot by providing a valid token to
// NewTelegramNotifier or you can load one from a save file.
// The method Stop allow for a graceful shutdown although it
// not wait for the bot to shut down before returning : some
// processing may still be ongoing.
package telegram

// FIXME there are thread-safety issues
// 	- the feed object is never locked on with read & write permitted
// 	- https://github.com/peterbourgon/diskv might be a better tool for storage

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maxime915/mk-giveaway-notifier/reddit"
	telegram "gopkg.in/tucnak/telebot.v2"
)

// default sub an user is subscribed to
const subreddit = "MechanicalKeyboards"

// savedState represent a configuration of the bot
type savedState struct {
	Token     string                 `json:"token"`
	Listeners map[int64]*reddit.Feed `json:"listener"`
}

// TelegramNotifier
type TelegramNotifier struct {
	*telegram.Bot
	redditBot *reddit.Bot
	savedState
	mutex   *sync.Mutex
	done    chan struct{}
	started bool
}

// newEmptyBot returns a new empty bot with valid mutex/done/Listeners using
// the default configuration to talk with reddit API.
func newEmptyBot() *TelegramNotifier {
	return &TelegramNotifier{
		savedState: savedState{Listeners: make(map[int64]*reddit.Feed)},
		mutex:      &sync.Mutex{},
		done:       make(chan struct{}), // dead channel
		started:    false,
	}
}

// NewTelegramNotifier returns a valid TelegramNotifier with the given token
func NewTelegramNotifier(Token string) (*TelegramNotifier, error) {
	return NewTelegramNotifierWithBot(Token, reddit.DefaultBot())
}

// NewTelegramNotifierWithBot returns a valid TelegramNotifier with the given token
// and using the given bot to call the reddit API.
func NewTelegramNotifierWithBot(Token string, rbot *reddit.Bot) (*TelegramNotifier, error) {
	bot, err := telegram.NewBot(telegram.Settings{
		Token:  Token,
		Poller: &telegram.LongPoller{Timeout: 30 * time.Second},
	})

	if err != nil {
		return nil, err
	}

	tgBot := newEmptyBot()
	tgBot.Bot = bot
	tgBot.redditBot = rbot
	tgBot.savedState.Token = Token

	return tgBot, nil
}

// LoadTelegramNotifier creates a bot from the configuration file using the default
// bot to communicate with reddit API.
func LoadTelegramNotifier(path string) (*TelegramNotifier, error) {
	return LoadTelegramNotifierWithBot(path, reddit.DefaultBot())
}

// LoadTelegramNotifierWithBot creates a bot from the configuration file using
// the given bot to communicate with reddit AOU
func LoadTelegramNotifierWithBot(path string, rbot *reddit.Bot) (*TelegramNotifier, error) {
	tgBot := newEmptyBot()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// sets Token & listeners
	err = json.Unmarshal(data, &tgBot.savedState)
	if err != nil {
		return nil, err
	}

	bot, err := telegram.NewBot(telegram.Settings{
		Token:  tgBot.savedState.Token,
		Poller: &telegram.LongPoller{Timeout: 30 * time.Second},
	})
	if err != nil {
		return nil, err
	}

	tgBot.Bot = bot
	tgBot.redditBot = rbot

	return tgBot, nil
}

// String represent the current state of the TelegramNotifier
func (tn *TelegramNotifier) String() string {
	tn.mutex.Lock()
	defer tn.mutex.Unlock()

	return fmt.Sprintf("%+v", tn.savedState)
}

// SaveTo prints the configuration of the TelegramNotifier to a file
func (tn *TelegramNotifier) SaveTo(path string) error {
	tn.mutex.Lock()
	defer tn.mutex.Unlock()

	data, err := json.Marshal(tn.savedState)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0777)
}

// add one chat to the listeners, returns false if the chat is already listening
func (b *TelegramNotifier) addListeners(chatID int64, feed *reddit.Feed) bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// avoid duplicate messages by keeping the list unique
	if _, ok := b.Listeners[chatID]; ok {
		return false
	}

	b.Listeners[chatID] = feed
	return true
}

// remove on chat from the listeners, returns false if chat was not listening
func (b *TelegramNotifier) removeListener(chatID int64) bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// make sure the key exist
	if _, ok := b.Listeners[chatID]; !ok {
		return false
	}

	delete(b.Listeners, chatID)
	return true
}

// Stop makes the bot stop listening to Telegram API. Ongoing requests will continue
// processing.
func (b *TelegramNotifier) Stop() {
	if b.started {
		b.Bot.Stop()
	}
	close(b.done)
}

// IsKilled returns true if the TelegramNotifier won't start any new processing
func (b *TelegramNotifier) IsKilled() bool {
	_, open := <-b.done
	return !open
}

// BlockUntilKilled wait until the TelegramNotifier receives a Stop() call, either
// via the function or via a Telegram message.
func (b *TelegramNotifier) BlockUntilKilled() {
	<-b.done
}

func isGiveaway(title string) bool {
	return strings.Contains(strings.ToLower(title), "giveaway")
}

// replyFetchedPosts
func (b *TelegramNotifier) replyFetchedPosts(m *telegram.Message, fetcher func(*reddit.Feed) ([]*reddit.Post, error)) error {
	return b.replyFilteredFetchedPosts(m, isGiveaway, fetcher)
}

// replyFilteredFetchedPosts creates a reply to the Sender of `m` using posts from
// `fetched` and filtering them via `filter`. Posts included are the one for which's
// filter(post) is true.
// The reply is split into a message per post and a confirmation reply. Each post
// is formatted to show the title, the author and give a permalink.
func (b *TelegramNotifier) replyFilteredFetchedPosts(m *telegram.Message, filter func(string) bool, fetcher func(*reddit.Feed) ([]*reddit.Post, error)) error {
	err := b.Notify(m.Sender, telegram.Typing)
	if err != nil {
		return err
	}

	feed, ok := b.Listeners[m.Chat.ID]
	if !ok {
		b.Send(m.Sender, "you are not subscribed to any feed")
		return nil
	}

	posts, err := fetcher(feed)
	if err != nil {
		b.Send(m.Sender, fmt.Sprintf("error while updating feed: %s", err.Error()))
		return err
	}

	if len(posts) == 0 {
		b.Send(m.Sender, "No post found yet, try again later")
		return nil
	}

	count := 0
	for _, post := range posts {
		if !filter(post.Title) {
			continue
		}
		count++
		_, err = b.Send(m.Sender, fmt.Sprintf(
			"%s by u/%s\nold.reddit.com%s",
			post.Title,
			post.Author,
			post.Permalink,
		))
		if err != nil {
			b.Send(m.Sender, "Error encountered while trying to send results")
			return err
		}
	}

	if len(posts) == 1 {
		comment := "It was not a giveaway."
		if count > 0 {
			comment = "It was a giveaway."
		}
		_, err = b.Send(m.Sender, fmt.Sprintf(
			"Fetched 1 post at *%s*.\n%s",
			posts[0].Created.Time.Local().Format(time.Stamp),
			comment,
		), "Markdown")
	} else {
		comment := "None of them were giveaways."
		if count == 1 {
			comment = "One of them was a giveaway."
		} else {
			comment = fmt.Sprintf("%d of them were giveaways.", count)
		}
		_, err = b.Send(m.Sender, fmt.Sprintf(
			"Fetched %d posts from *%s* to *%s*.\n%s",
			len(posts),
			posts[len(posts)-1].Created.Time.Local().Format(time.Stamp),
			posts[0].Created.Time.Local().Format(time.Stamp),
			comment,
		), "Markdown")
	}

	return err
}

// Launch starts the bot and blocks until Stop() is called or the bot
// receives a message requesting halt.
func (b *TelegramNotifier) Launch() error {
	errChan := make(chan error)

	b.Handle("/ping", func(m *telegram.Message) {
		err := b.Notify(m.Sender, telegram.Typing)
		if err != nil {
			errChan <- err
			return
		}

		_, err = b.Send(m.Sender, "Hello World!")
		if err != nil {
			errChan <- err
			return
		}
	})

	b.Handle("/subscribe", func(m *telegram.Message) {
		err := b.Notify(m.Sender, telegram.Typing)
		if err != nil {
			errChan <- err
			return
		}

		feed, err := b.redditBot.NewFeed(subreddit)
		if err != nil {
			b.Send(m.Sender, "Internal error: you won't be able to /poll or /update")
			errChan <- err
			return
		}

		added := b.addListeners(m.Chat.ID, feed)

		if added {
			_, err = b.Send(m.Sender, "Noted, you are now listening on mk-giveaway-notifier")
		} else {
			_, err = b.Send(m.Sender, "This is not the bot you are looking for (you already listen to mk-giveaway-notifier)")
		}

		if err != nil {
			errChan <- err
		}
	})

	b.Handle("/unsubscribe", func(m *telegram.Message) {
		removed := b.removeListener(m.Chat.ID)
		var err error

		if removed {
			_, err = b.Send(m.Sender, "You are no longer receiving update")
		} else {
			_, err = b.Send(m.Sender, "You are not registered yet")
		}

		if err != nil {
			errChan <- err
		}
	})

	b.Handle("/kill", func(m *telegram.Message) {
		_, err := b.Send(m.Sender, "Goodbye!")
		if err != nil {
			errChan <- err
		}
		b.Stop()
	})

	b.Handle("K", func(m *telegram.Message) {
		_, err := b.Send(m.Sender, "Goodbye!")
		if err != nil {
			errChan <- err
		}
		b.Stop()
	})

	b.Handle("/touch", func(m *telegram.Message) {
		err := b.replyFetchedPosts(m, b.redditBot.Touch)
		if err != nil {
			errChan <- err
		}
	})

	updateHandle := func(m *telegram.Message) {
		err := b.replyFetchedPosts(m, b.redditBot.Update)
		if err != nil {
			errChan <- err
		}
	}

	b.Handle("/update", updateHandle)
	b.Handle("/up", updateHandle)

	b.Handle("/grow", func(m *telegram.Message) {
		size, err := strconv.Atoi(m.Payload)
		if err != nil || size < 1 {
			_, err := b.Send(m.Sender, "/grow requires positive size")
			if err != nil {
				errChan <- err
			}
			return
		}

		err = b.replyFetchedPosts(m, func(f *reddit.Feed) ([]*reddit.Post, error) {
			return b.redditBot.UpdateForAnchorSize(f, size)
		})
		if err != nil {
			errChan <- err
		}

		_, err = b.Send(m.Sender, fmt.Sprintf("Anchor size is now %d", size))
		if err != nil {
			errChan <- err
		}
	})

	b.Handle("/peek", func(m *telegram.Message) {
		err := b.replyFetchedPosts(m, b.redditBot.Peek)
		if err != nil {
			errChan <- err
		}
	})

	b.Handle("/poll", func(m *telegram.Message) {
		_, err := b.Send(m.Sender, "unsupported yet")
		if err != nil {
			errChan <- err
		}
	})

	b.Handle("/debug", func(m *telegram.Message) {
		data, _ := json.Marshal(b.Listeners)

		log.Println(string(data))
		_, err := b.Send(m.Sender, string(data))
		if err != nil {
			errChan <- err
		}
	})

	go b.Start()
	b.started = true

	for {
		select {
		case err := <-errChan:
			log.Println(err)
		case <-b.done:
			return nil
		}
	}
}
