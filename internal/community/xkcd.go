package community

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/go-resty/resty/v2"

	"github.com/Neon-Genesis-Linux/pen-bot/internal/core"
	"github.com/Neon-Genesis-Linux/pen-bot/internal/messaging"
)

const xkcdCommand = "xkcd"

var xkcdClient = resty.New().
	SetTimeout(10 * time.Second).
	SetRedirectPolicy(resty.NoRedirectPolicy())

type xkcdMetadata struct {
	Title      string `json:"title"`
	Num        int    `json:"num"`
	Img        string `json:"img"`
	Year       string `json:"year"`
	Month      string `json:"month"`
	Day        string `json:"day"`
	Transcript string `json:"transcript"`
	Alt        string `json:"alt"`
	Link       string `json:"link"`
	News       string `json:"news"`
	SafeTitle  string `json:"safe_title"`
}

func registerXkcdCommands() {
	// Commands
	core.RegisterCommand(xkcdCommand, handleXkcdSpecific)
	core.RegisterCommandPath([]string{xkcdCommand, "random"}, handleXkcdRandom)
	core.RegisterCommandPath([]string{xkcdCommand, "r"}, handleXkcdRandom)
	core.RegisterCommandPath([]string{xkcdCommand, "latest"}, handleXkcdCurrent)
	core.RegisterCommandPath([]string{xkcdCommand, "l"}, handleXkcdCurrent)
	core.RegisterCommandPath([]string{xkcdCommand, "current"}, handleXkcdCurrent)
	core.RegisterCommandPath([]string{xkcdCommand, "c"}, handleXkcdCurrent)

	// Aliases
	core.RegisterAlias("xk", xkcdCommand)
}

func getXkcdMetadata(numStr string) (xkcdMetadata, error) {
	var query string
	if numStr != "" {
		if _, err := strconv.Atoi(numStr); err != nil {
			return xkcdMetadata{}, fmt.Errorf("invalid comic number: %q", numStr)
		}
		query = numStr
	}

	var metadata xkcdMetadata
	resp, err := xkcdClient.R().SetResult(&metadata).Get(fmt.Sprintf("https://xkcd.com/%s/info.0.json", query))
	if err != nil {
		slog.Error("XKCD: Connection error", slog.Any("error", err))
		return xkcdMetadata{}, err
	}
	if !resp.IsSuccess() {
		return xkcdMetadata{}, fmt.Errorf("XKCD: unexpected status %d", resp.StatusCode())
	}
	return metadata, nil
}

func handleXkcdCurrent(event *events.MessageCreate, _ []string) {
	metadata, err := getXkcdMetadata("")
	if err != nil {
		_ = messaging.SendReply(event, "Unable to fetch metadata for current xkcd")
		return
	}
	sendXkcd(event, metadata)
}

func handleXkcdSpecific(event *events.MessageCreate, args []string) {
	if len(args) == 0 {
		handleXkcdCurrent(event, []string{})
		return
	}
	if _, err := strconv.Atoi(args[0]); err != nil {
		_ = messaging.SendReply(event, "Parameter must be a valid integer.")
		return
	}
	metadata, err := getXkcdMetadata(args[0])
	if err != nil {
		_ = messaging.SendReply(event, fmt.Sprintf("Unable to fetch metadata for xkcd #%s", args[0]))
		return
	}
	sendXkcd(event, metadata)
}

func handleXkcdRandom(event *events.MessageCreate, _ []string) {
	resp, err := xkcdClient.R().Get("https://c.xkcd.com/random/comic/")
	if err != nil {
		slog.Error("XKCD: random fetch error", slog.Any("error", err))
		_ = messaging.SendReply(event, "Unable to fetch random xkcd")
		return
	}

	if resp.StatusCode() != http.StatusFound {
		slog.Error("XKCD: unexpected status", slog.Int("status", resp.StatusCode()))
		_ = messaging.SendReply(event, "Unexpected response from xkcd")
		return
	}

	loc := resp.Header().Get("Location")
	if loc == "" {
		_ = messaging.SendReply(event, "No redirect location found")
		return
	}

	loc = strings.TrimSuffix(loc, "/")
	numStr := loc[strings.LastIndex(loc, "/")+1:]

	if _, err := strconv.Atoi(numStr); err != nil {
		slog.Error("XKCD: failed to parse comic number", slog.String("location", loc))
		_ = messaging.SendReply(event, "Failed to parse redirected comic number")
		return
	}

	metadata, err := getXkcdMetadata(numStr)
	if err != nil {
		_ = messaging.SendReply(event, fmt.Sprintf("Unable to fetch metadata for xkcd #%s", numStr))
		return
	}
	sendXkcd(event, metadata)
}

func sendXkcd(event *events.MessageCreate, metadata xkcdMetadata) {
	embed := discord.NewEmbed().
		WithTitle(metadata.Title).
		WithURL(fmt.Sprintf("https://xkcd.com/%d", metadata.Num)).
		WithDescription(metadata.Alt).
		WithImage(metadata.Img).
		WithFooter(fmt.Sprintf("xkcd #%d — %s/%s/%s", metadata.Num, metadata.Month, metadata.Day, metadata.Year), "").
		WithColor(0x96A8C8)
	builder := discord.NewMessageCreate().
		WithMessageReference(&discord.MessageReference{MessageID: &event.Message.ID}).
		AddEmbeds(embed)
	_, _ = event.Client().Rest.CreateMessage(event.ChannelID, builder)
}
