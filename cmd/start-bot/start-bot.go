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

func main() {
	log.SetFlags(log.LstdFlags | log.Llongfile)

	token := flag.String("token", "", "Telegram token (required)")
	path := flag.String("db", "", "Path to the database file (required, will be created if file doesn't exist)")
	flag.Parse()

	if len(*path) == 0 {
		log.Fatal("database file store is required")
	}
	if len(*token) == 0 {
		log.Fatal("telegram token is required")
	}

	// listen to interrupts
	interrupted := make(chan struct{})
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-c
		close(interrupted)
	}()

	// bot creation
	bot, err := telegram.NewTelegramNotifier(*token, *path)
	if err != nil {
		log.Fatalf("unable to start: %s\nIf you are online, verify the token\n", err.Error())
	}

	// start telegram bot
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
}
