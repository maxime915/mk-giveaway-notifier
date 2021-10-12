// telegram handles the reception/reply of messages.
// You can create a new bot by providing a valid token to
// NewTelegramNotifier or you can load one from a save file.
// The method Stop allow for a graceful shutdown although it
// not wait for the bot to shut down before returning : some
// processing may still be ongoing.
package telegram

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/maxime915/mk-giveaway-notifier/reddit"
	bolt "go.etcd.io/bbolt"
	telegram "gopkg.in/tucnak/telebot.v2"
)

// default sub an user is subscribed to
const (
	subreddit  = "MechanicalKeyboards"
	bucketName = "main-bucket" // one global bucket
)

// TelegramNotifier
type TelegramNotifier struct {
	*telegram.Bot
	redditBot *reddit.Bot
	db        *bolt.DB
	done      chan struct{}
	started   bool
}

// newEmptyBot returns a new empty bot (properties to be filled up)
func newEmptyBot() *TelegramNotifier {
	return &TelegramNotifier{
		done:    make(chan struct{}), // dead channel
		started: false,
	}
}

// NewTelegramNotifier returns a valid TelegramNotifier with the given token
func NewTelegramNotifier(Token, DBPath string) (*TelegramNotifier, error) {
	return NewTelegramNotifierWithBot(Token, DBPath, reddit.DefaultBot())
}

// NewTelegramNotifierWithBot returns a valid TelegramNotifier with the given token
// and using the given bot to call the reddit API.
func NewTelegramNotifierWithBot(Token, DBPath string, redditBot *reddit.Bot) (*TelegramNotifier, error) {
	bot, err := telegram.NewBot(telegram.Settings{
		Token:  Token,
		Poller: &telegram.LongPoller{Timeout: 30 * time.Second},
	})

	if err != nil {
		return nil, err
	}

	tgBot := newEmptyBot()
	tgBot.Bot = bot
	tgBot.redditBot = redditBot
	tgBot.db, err = bolt.Open(DBPath, 0666, nil)

	if err != nil {
		return nil, err
	}

	return tgBot, nil
}

// String represent the current state of the TelegramNotifier
func (b *TelegramNotifier) String() string {
	data := make(map[int64]*reddit.Feed)

	err := b.db.View(func(t *bolt.Tx) error {
		bucket := t.Bucket([]byte(bucketName))

		cursor := bucket.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			key := int64(binary.BigEndian.Uint64(k))

			var feed *reddit.Feed
			err := json.Unmarshal(v, &feed)
			if err != nil {
				return err
			}

			data[key] = feed
		}
		return nil
	})

	if err != nil {
		log.Println(err)
		return "A TelegramNotifier with a least 1 error (see logs.)"
	}

	payload, err := json.Marshal(data)
	if err != nil {
		log.Println(err)
		return "A TelegramNotifier with a least 1 error (see logs.)"
	}

	return string(payload)
}

// add one chat to the listeners, returns false if the chat is already listening
func (b *TelegramNotifier) addListeners(chatID int64, feed *reddit.Feed) error {
	return b.db.Update(func(t *bolt.Tx) error {
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, uint64(chatID))

		bucket := t.Bucket([]byte(bucketName))

		data, err := json.Marshal(feed)
		if err != nil {
			return err
		}

		if check := bucket.Get(key); check != nil {
			return KeyExistError{}
		}

		err = bucket.Put(key, data)

		return err
	})
}

// remove on chat from the listeners, returns false if chat was not listening
func (b *TelegramNotifier) removeListener(chatID int64) error {
	return b.db.Update(func(t *bolt.Tx) error {
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, uint64(chatID))

		bucket := t.Bucket([]byte(bucketName))

		if check := bucket.Get(key); check == nil {
			return KeyNotFoundError{}
		}

		return bucket.Delete(key)
	})
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

	var posts []*reddit.Post = nil

	err = b.db.Update(func(t *bolt.Tx) error {
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, uint64(m.Chat.ID))

		bucket := t.Bucket([]byte(bucketName))

		data := bucket.Get(key)

		if key == nil {
			return KeyNotFoundError{}
		}

		var feed *reddit.Feed
		err = json.Unmarshal(data, &feed)

		if err != nil {
			return err
		}

		posts, err = fetcher(feed)
		if err != nil {
			return err
		}

		// fetcher may have modified feed, the new value should be stored
		data, err = json.Marshal(feed)
		if err != nil {
			return err
		}

		return bucket.Put(key, data)
	})

	switch err.(type) {
	case KeyNotFoundError:
		b.Send(m.Sender, "You are not subscribed to any feed.")
		return nil
	case reddit.EmptyAnchorError:
		b.Send(m.Sender, "The feed has no anchor, /touch it before fetching it")
		return nil
	default:
		b.Send(m.Sender, fmt.Sprintf("error while updating feed: %s", err.Error()))
		return err
	case nil:
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

	// create a global bucket
	if err := b.db.Update(func(t *bolt.Tx) error {
		_, err := t.CreateBucketIfNotExists([]byte(bucketName))
		return err
	}); err != nil {
		return err
	}

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
			b.Send(m.Sender, "Internal error, please re-try later (is your internet connection ok?)")
			errChan <- err
			return
		}

		err = b.addListeners(m.Chat.ID, feed)
		switch err.(type) {
		case nil:
			_, err = b.Send(m.Sender, "Noted, you are now listening on mk-giveaway-notifier.")
		case KeyExistError:
			_, err = b.Send(m.Sender, "You already listen to mk-giveaway-notifier.")
		default:
			b.Send(m.Sender, "Unable to subscribe, see logs for detail.")
		}

		if err != nil {
			errChan <- err
		}
	})

	b.Handle("/unsubscribe", func(m *telegram.Message) {
		err := b.removeListener(m.Chat.ID)

		switch err.(type) {
		case nil:
			_, err = b.Send(m.Sender, "You are no longer receiving update")
		case KeyNotFoundError:
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
		message := b.String()

		log.Println(message)
		_, err := b.Send(m.Sender, message)
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
