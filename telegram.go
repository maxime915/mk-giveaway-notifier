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
	Listeners []int64
	mutex     sync.Mutex
}

func NewTelegramNotifier(data TelegramData) (*TelegramNotifier, error) {
	bot, err := telebot.NewBot(telebot.Settings{
		Token:  data.Token,
		Poller: &telebot.LongPoller{Timeout: delay},
	})

	if err != nil {
		return nil, err
	}

	return &TelegramNotifier{Bot: bot, Listeners: data.Listeners}, nil
}

func (b *TelegramNotifier) SaveListeners(data *TelegramData) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	data.Listeners = b.Listeners
}

// add one chat to the listeners, returns false if the chat is already listening
func (b *TelegramNotifier) addListeners(chatID int64) bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// avoid duplicate messages by keeping the list unique
	for i := range b.Listeners {
		if b.Listeners[i] == chatID {
			return false
		}
	}

	b.Listeners = append(b.Listeners, chatID)
	return true
}

// TODO actual notification
func (b *TelegramNotifier) NotifyAll(post *reddit.Post) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, chat := range b.Listeners {
		b.Send(telebot.ChatID(chat), "you have been notified")
	}
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

	// TODO unsubscribe

	b.Handle("/kill", func(m *telebot.Message) {
		b.Send(m.Sender, "where were u wen club penguin die\ni was at house eating dorito when phone ring\n\"Club penguin is kil\"\n\"no\"")
		b.Stop()
	})

	b.Handle(telebot.OnText, func(m *telebot.Message) {
		b.Send(m.Sender, "I did not quite understand that")
	})

	b.Start()
}
