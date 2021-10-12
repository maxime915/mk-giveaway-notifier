// start-bot: CLI to launch the telegram & reddit bots.
// Usage of start-bot:
//   -path string
//         Path to load/save the configuration of the bot: if token is given this file will be erased (default "telegram_state.private.json")
//   -token string
//         Telegram bot token: if given, a new bot is created and the path is only used to save the configuration
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/maxime915/mk-giveaway-notifier/telegram"
)

const savedStatePath = "telegram_state.private.json"

func main() {
	log.SetFlags(log.LstdFlags | log.Llongfile)

	token := flag.String("token", "", "Telegram bot token: if given, a new bot is created and the path is only used to save the configuration")
	path := flag.String("path", savedStatePath, "Path to load/save the configuration of the bot: if token is given this file will be erased")
	flag.Parse()

	if len(*path) == 0 {
		log.Fatalf("invalid path to store the state of the bot")
	}

	interrupted := make(chan struct{})

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		<-c
		close(interrupted)
	}()

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

	done := make(chan struct{})
	go func() {
		err = bot.Launch()
		if err != nil {
			log.Fatalf("internal error: %s\n", err.Error())
		}
		close(done)
	}()

	// when interrupted, stop and wait until done
	// when done, proceed
	select {
	case <-interrupted:
		bot.Stop()
	case <-done:
	}

	err = bot.SaveTo(*path)
	if err != nil {
		log.Fatalf("internal error while trying to save state: %s\n"+
			"state to save: %s\n", err.Error(), bot.String())
	}
}
