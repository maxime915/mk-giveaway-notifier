# mk-giveaway-notifier

[![Go Reference](https://pkg.go.dev/badge/github.com/maxime915/mk-giveaway-notifier.svg)](https://pkg.go.dev/github.com/maxime915/mk-giveaway-notifier)
<!-- ![AppVeyor](https://img.shields.io/appveyor/build/maxime915/mk-giveaway-notifier) -->

2-in-1 bot to search for giveaway in r/mk and notify me on telegram

# mk-giveaway-notifier/cmd/start-bot

## Installation
`go install github.com/maxime915/mk-giveaway-notifier/cmd/start-bot@latest`

## Usage

```
Usage of start-bot:
  -path string
        Path to load/save the configuration of the bot: if token is given this file will be erased (default "telegram_state.private.json")
  -token string
        Telegram bot token: if given, a new bot is created and the path is only used to save the configuration
```

