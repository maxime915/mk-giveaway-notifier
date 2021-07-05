package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

const (
	dataPath = "saved_state.private.json"
)

type SavedState struct {
	TelegramData
	RedditData
}

func main() {
	data, err := os.ReadFile(dataPath)
	if err != nil {
		log.Fatal("could not find '" + dataPath + "' on your system, did you build a saved state?")
	}

	var state SavedState
	err = json.Unmarshal(data, &state)
	if err != nil {
		log.Fatal("unable to load the configuration file: ", err)
	}

	feed, err := NewFeedFromData(state.RedditData)
	if err != nil {
		log.Fatal("unable to fetch reddit: ", err)
	}

	telegram, err := NewTelegramNotifier(state.TelegramData)
	if err != nil {
		log.Fatal("unable to launch telegram bot: ", err)
	}

	go func() {
		for post := range feed.Post {
			telegram.NotifyAll(post)
		}

		for err := range feed.Errs {
			fmt.Println(err)
		}
	}()

	telegram.Launch() // halts until /kill is received
	feed.Kill <- true // stop reddit too

	telegram.SaveListeners(&state.TelegramData)
	feed.Update(&state.RedditData)

	data, err = json.Marshal(state)
	err = os.WriteFile(dataPath, data, os.ModePerm)
}
