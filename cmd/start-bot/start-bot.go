package main

import (
	"flag"
	"log"

	"github.com/maxime915/mk-giveaway-notifier/telegram"
)

const savedStatePath = "telegram_state.private.json"

func main() {
	token := flag.String("token", "", "Telegram bot token")
	path := flag.String("path", savedStatePath, "Path to save the state of the bot to")
	flag.Parse()

	if len(*path) == 0 {
		log.Fatalf("invalid path to store the state of the bot")
	}

	var bot *telegram.TelegramNotifier
	var err error

	if len(*token) > 0 {
		bot, err = telegram.NewTelegramNotifier(*token)
	} else {
		bot, err = telegram.LoadTelegramNotifier(*path)
	}

	if err != nil {
		log.Fatalf("unable to start: %s\nIf you are online, verify the token\n", err.Error())
	}

	err = bot.Launch()
	if err != nil {
		log.Fatalf("internal error: %s\n", err.Error())
	}

	err = bot.SaveTo(*path)
	if err != nil {
		log.Fatalf("internal error while trying to save state: %s\n"+
			"state to save: %s\n", err.Error(), bot.String())
	}
}