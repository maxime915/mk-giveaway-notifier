package telegram

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/maxime915/mk-giveaway-notifier/reddit"
	telegram "gopkg.in/tucnak/telebot.v2"
)

// default sub an user is subscribed to
const subbredit = "MechanicalKeyboards"

type savedState struct {
	Token     string                 `json:"token"`
	Listeners map[int64]*reddit.Feed `json:"listener"`
}

type TelegramNotifier struct {
	*telegram.Bot
	redditBot *reddit.Bot
	savedState
	mutex *sync.Mutex
	Done  chan struct{}
}

func newEmptyBot() *TelegramNotifier {
	return &TelegramNotifier{
		savedState: savedState{Listeners: make(map[int64]*reddit.Feed)},
		mutex:      &sync.Mutex{},
		Done:       make(chan struct{}), // dead channel
	}
}

func NewTelegramNotifier(Token string) (*TelegramNotifier, error) {
	return NewTelegramNotifierWithBot(Token, reddit.DefaultBot())
}

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

func LoadTelegramNotifier(path string) (*TelegramNotifier, error) {
	return LoadTelegramNotifierWithBot(path, reddit.DefaultBot())
}

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
	return fmt.Sprintf("%+v", tn.savedState)
}

func (tn *TelegramNotifier) SaveTo(path string) error {
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

func (b *TelegramNotifier) Stop() {
	b.Bot.Stop()
	close(b.Done)
}

func isGiveaway(title string) bool {
	return strings.Contains(strings.ToLower(title), "giveaway")
}

func (b *TelegramNotifier) FetchPosts(m *telegram.Message, fetcher func(*reddit.Feed) ([]*reddit.Post, error)) {
	b.FetchPostsAndFilter(m, isGiveaway, fetcher)
}

func (b *TelegramNotifier) FetchPostsAndFilter(m *telegram.Message, predicate func(string) bool, fetcher func(*reddit.Feed) ([]*reddit.Post, error)) {
	b.Notify(m.Sender, telegram.Typing)

	feed, ok := b.Listeners[m.Chat.ID]
	if !ok {
		b.Send(m.Sender, "you are not subscribed to any feed")
		return
	}

	posts, err := fetcher(feed)
	if err != nil {
		b.Send(m.Sender, fmt.Sprintf("error while updating feed: %s", err.Error()))
		return
	}

	if len(posts) == 0 {
		b.Send(m.Sender, "No post found yet, try again later")
		return
	}

	for _, post := range posts {
		if !predicate(post.Title) {
			continue
		}
		b.Send(m.Sender, fmt.Sprintf(
			"%s by u/%s\nold.reddit.com%s",
			post.Title,
			post.Author,
			post.Permalink,
		))
	}

	b.Send(m.Sender, "And that's it for now!")
}

// TODO handle errors
func (b *TelegramNotifier) Launch() error {

	b.Handle("/hello", func(m *telegram.Message) {
		b.Send(m.Sender, "Hello World!")
	})

	b.Handle("/subscribe", func(m *telegram.Message) {
		b.Notify(m.Sender, telegram.Typing)

		feed, err := b.redditBot.NewFeed(subbredit)
		if err != nil {
			b.Send(m.Sender, "Internal error: you won't be able to /poll or /update")
			return
		}

		added := b.addListeners(m.Chat.ID, feed)

		if added {
			b.Send(m.Sender, "Noted, you are now listening on mk-giveaway-notifier")
		} else {
			b.Send(m.Sender, "This is not the bot you are looking for (you already listen to mk-giveaway-notifier)")
			return
		}
	})

	b.Handle("/unsubscribe", func(m *telegram.Message) {
		removed := b.removeListener(m.Chat.ID)
		if removed {
			b.Send(m.Sender, "You are no longer receiving update")
		} else {
			b.Send(m.Sender, "You are not registered yet")
		}
	})

	b.Handle("/kill", func(m *telegram.Message) {
		b.Send(m.Sender, "where were u wen club penguin die\ni was at house eating dorito when phone ring\n\"Club penguin is kil\"\n\"no\"")
		// b.Stop()
	})

	b.Handle("K", func(m *telegram.Message) {
		b.Delete(m)
		b.Stop()
	})

	b.Handle("/touch", func(m *telegram.Message) {
		b.Notify(m.Sender, telegram.Typing)

		feed, ok := b.Listeners[m.Chat.ID]
		if !ok {
			b.Send(m.Sender, "you are not subscribed to any feed")
			return
		}

		err := b.redditBot.Touch(feed)
		if err != nil {
			b.Send(m.Sender, fmt.Sprintf("error while touch'ing feed: %s", err.Error()))
		} else {
			b.Send(m.Sender, "All fine!")
		}
	})

	b.Handle("/update", func(m *telegram.Message) {
		b.FetchPosts(m, b.redditBot.Update)
	})

	b.Handle("/peek", func(m *telegram.Message) {
		b.FetchPosts(m, b.redditBot.Peek)
	})

	b.Handle("/poll", func(m *telegram.Message) {
		b.Send(m.Sender, "got:"+m.Text)
	})

	b.Handle(telegram.OnText, func(m *telegram.Message) {
		b.Send(m.Sender, "I did not quite understand that")
	})

	b.Start()
	return nil
}
