package telegram

import (
	"strings"
)

type baseError struct{}

func (baseError) Error() string { return "" }

type KeyExistError struct{ baseError }

type KeyNotFoundError struct{ baseError }

func isGiveaway(title string) bool {
	return strings.Contains(strings.ToLower(title), "giveaway")
}
