package config

import (
	"os"
	"strings"

	"github.com/disgoorg/snowflake/v2"
)

const (
	// botOwnerEnv names the variable a comma-separated list of bot owners is read
	// from, so the owner above the guild stays configurable without a REST call.
	botOwnerEnv = "BOT_OWNER"
)

// BotOwners returns the set of owners the bot is configured to be, read from the
// BOT_OWNER env var as a comma-separated list of snowflakes. A value that names
// no parseable id yields an empty set rather than a nil one, so a caller can
// range it and the membership test never dereferences a nil map.
func BotOwners() map[snowflake.ID]struct{} {
	owners := map[snowflake.ID]struct{}{}
	for _, raw := range strings.Split(os.Getenv(botOwnerEnv), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if id, err := snowflake.Parse(raw); err == nil {
			owners[id] = struct{}{}
		}
	}
	return owners
}
