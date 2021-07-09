package main

import (
	"sync"

	"github.com/turnage/graw/reddit"
	"gopkg.in/tucnak/telebot.v2"
)

type TelegramData struct {
	Token     string  `json:"token"`
	Listeners []int64 `json:"listener"`
}

type TelegramNotifier struct {
	*telebot.Bot
	listeners map[int64]struct{}
	mutex     sync.Mutex
	Done      chan struct{}
}

func NewTelegramNotifier(data TelegramData) (*TelegramNotifier, error) {
	bot, err := telebot.NewBot(telebot.Settings{
		Token:  data.Token,
		Poller: &telebot.LongPoller{Timeout: delay},
	})

	if err != nil {
		return nil, err
	}

	tgBot := &TelegramNotifier{
		Bot:       bot,
		listeners: make(map[int64]struct{}, len(data.Listeners)),
		Done:      make(chan struct{}), // dead channel
	}

	for _, l := range data.Listeners {
		tgBot.listeners[l] = struct{}{}
	}

	return tgBot, nil
}

func (b *TelegramNotifier) SaveListeners(data *TelegramData) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	data.Listeners = make([]int64, len(b.listeners))
	pos := 0
	for chat := range b.listeners {
		data.Listeners[pos] = chat
		pos++
	}
}

// add one chat to the listeners, returns false if the chat is already listening
func (b *TelegramNotifier) addListeners(chatID int64) bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// avoid duplicate messages by keeping the list unique
	if _, ok := b.listeners[chatID]; ok {
		return false
	}

	b.listeners[chatID] = struct{}{}
	return true
}

func (b *TelegramNotifier) removeListener(chatID int64) bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// make sure the key exist
	if _, ok := b.listeners[chatID]; !ok {
		return false
	}

	delete(b.listeners, chatID)
	return true
}

// TODO actual notification
func (b *TelegramNotifier) NotifyAll(post *reddit.Post) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for chat := range b.listeners {
		b.Send(telebot.ChatID(chat), "u/"+post.Author+" just posted "+post.Title+" in r/"+post.Subreddit)
	}
}

func (b *TelegramNotifier) Stop() {
	b.Bot.Stop()
	close(b.Done)
}

func (b *TelegramNotifier) Launch() {
	b.Handle("/hello", func(m *telebot.Message) {
		b.Send(m.Sender, "Hello World!")
	})

	b.Handle("/subscribe", func(m *telebot.Message) {
		b.Notify(m.Sender, telebot.Typing)

		added := b.addListeners(m.Chat.ID)

		if added {
			b.Send(m.Sender, "Noted, you are now listening on mk-giveaway-notifier")
		} else {
			b.Send(m.Sender, "This is not the bot you are looking for (you already listen to mk-giveaway-notifier)")
		}
	})

	b.Handle("/unsubscribe", func(m *telebot.Message) {
		removed := b.removeListener(m.Chat.ID)
		if removed {
			b.Send(m.Sender, "You are no longer receiving update")
		} else {
			b.Send(m.Sender, "You are not registered yet")
		}
	})

	b.Handle("/kill", func(m *telebot.Message) {
		b.Send(m.Sender, "where were u wen club penguin die\ni was at house eating dorito when phone ring\n\"Club penguin is kil\"\n\"no\"")
		b.Stop()
	})

	b.Handle("K", func(m *telebot.Message) {
		b.Stop()
	})

	b.Handle(telebot.OnText, func(m *telebot.Message) {
		b.Send(m.Sender, "I did not quite understand that")
	})

	b.Start()
}
