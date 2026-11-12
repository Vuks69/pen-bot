package config

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/Neon-Genesis-Linux/pen-bot/internal/core"
)

const (
	configCommand         = "config"
	configDescription     = "Read and change which role acts as admin or mod"
	viewSubcommand        = "view"
	rolesSubcommand       = "roles"
	setAdminRoleCommand   = "set-admin-role"
	clearAdminRoleCommand = "clear-admin-role"
	setModRoleCommand     = "set-mod-role"
	clearModRoleCommand   = "clear-mod-role"
	rolesOption           = "role"

	// Discord accepts at most this many characters in a message.
	messageContentLimit = 2000
)

const (
	configGuildOnlyMessage = "This command only works inside a server."
	configDeniedMessage    = "Only this server's owner can read or change these settings."
	configReadFailed       = "The settings could not be read."
	configWriteFailed      = "The settings could not be saved."
	configBadRoleMessage   = "That option did not name a role this server has."
	configNoRoleMessage    = "There is no admin or mod role set to clear."
)

// roleOption is the shared role parameter every set subcommand takes: Discord
// syncs a command's choices once at startup, so a role is a bare reference rather
// than a choice, and the handler accepts any live role and rejects the rest.
var roleOption = discord.ApplicationCommandOptionRole{
	Name:        rolesOption,
	Description: "The role to set, or a role this server has",
	Required:    true,
}

// Register adds the /config command and the guild-wide settings scope. Call it
// before Start, like every other registration: commands registered afterwards are
// never synced to Discord.
func Register() error {
	if err := registerGuildScope(); err != nil {
		return err
	}

	core.RegisterCommands(configCommandCreate())

	h := core.Mux()
	h.Route("/"+configCommand, func(r handler.Router) {
		r.SlashCommand("/"+viewSubcommand, handleConfigView)
		r.SlashCommand("/"+rolesSubcommand, handleConfigRoles)
		r.SlashCommand("/"+setAdminRoleCommand, handleConfigSetAdminRole)
		r.SlashCommand("/"+clearAdminRoleCommand, handleConfigClearAdminRole)
		r.SlashCommand("/"+setModRoleCommand, handleConfigSetModRole)
		r.SlashCommand("/"+clearModRoleCommand, handleConfigClearModRole)
	})
	return nil
}

// configCommandCreate describes /config. Its default member permissions are left
// unset on purpose: the command gates on the guild owner rather than on any role,
// so who may run it is decided entirely by the lockout gate in requireAdmin.
func configCommandCreate() discord.SlashCommandCreate {
	return discord.SlashCommandCreate{
		Name:        configCommand,
		Description: configDescription,
		Contexts:    []discord.InteractionContextType{discord.InteractionContextTypeGuild},
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionSubCommand{
				Name:        viewSubcommand,
				Description: "Show every setting scope this bot knows about",
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        rolesSubcommand,
				Description: "Show which role this server has set as admin or mod",
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        setAdminRoleCommand,
				Description: "Set the role that acts as admin",
				Options:     []discord.ApplicationCommandOption{discord.ApplicationCommandOption(roleOption)},
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        clearAdminRoleCommand,
				Description: "Clear the admin role",
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        setModRoleCommand,
				Description: "Set the role that acts as mod",
				Options:     []discord.ApplicationCommandOption{discord.ApplicationCommandOption(roleOption)},
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        clearModRoleCommand,
				Description: "Clear the mod role",
			},
		},
	}
}

const noRoleConfigured = "*no role set*"

func handleConfigView(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	_, settings, err := requireAdmin(e)
	if err != nil {
		return err
	}

	count := configuredCount(settings)
	scopes := Scopes()
	lines := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		lines = append(lines, fmt.Sprintf("`%s` **%s** — %s", scope.Key, scope.Module, scope.Summary))
	}

	embed := discord.NewEmbed().
		WithTitle("Configuration").
		WithDescription(strings.Join(lines, "\n")).
		WithFooter(fmt.Sprintf("%d of 2 roles configured, see /%s %s", count, configCommand, rolesSubcommand), "")
	return ephemeralEmbed(e, embed)
}

// configuredCount is how many of the two role slots a guild has filled.
func configuredCount(settings *GuildSettings) int {
	count := 0
	if settings.AdminRole != nil {
		count++
	}
	if settings.ModRole != nil {
		count++
	}
	return count
}

// handleConfigRoles shows which role each slot holds, or that the slot is unset.
func handleConfigRoles(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	_, settings, err := requireAdmin(e)
	if err != nil {
		return err
	}

	embed := discord.NewEmbed().WithTitle("Configured Roles")
	embed = addRoleField(embed, "Admin", settings.AdminRole)
	embed = addRoleField(embed, "Mod", settings.ModRole)
	return ephemeralEmbed(e, embed)
}

// addRoleField is one row /config roles shows: a role mention when a role is set,
// or the *no role set* placeholder when it is not.
func addRoleField(embed discord.Embed, label string, role *string) discord.Embed {
	if role == nil {
		return embed.AddField(label, noRoleConfigured, false)
	}
	return embed.AddField(label, "<@&"+*role+">", false)
}

func handleConfigSetAdminRole(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	guildID, _, err := requireAdmin(e)
	if err != nil {
		return err
	}
	roleID, err := optionRoleID(data)
	if err != nil {
		slog.Warn("could not read the role option", "guild_id", guildID, slog.Any("error", err))
		return ephemeralReply(e, configBadRoleMessage)
	}
	if err := SetAdminRole(e.Ctx, guildID, roleID); err != nil {
		slog.Warn("could not set the admin role", "guild_id", guildID, "role_id", roleID, slog.Any("error", err))
		return ephemeralReply(e, configWriteFailed)
	}
	return ephemeralReply(e, "The admin role is now "+roleMention(roleID)+".")
}

func handleConfigClearAdminRole(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	guildID, _, err := requireAdmin(e)
	if err != nil {
		return err
	}
	removed, err := ClearAdminRole(e.Ctx, guildID)
	if err != nil {
		slog.Warn("could not clear the admin role", "guild_id", guildID, slog.Any("error", err))
		return ephemeralReply(e, configWriteFailed)
	}
	if !removed {
		return ephemeralReply(e, configNoRoleMessage)
	}
	return ephemeralReply(e, "Cleared the admin role.")
}

func handleConfigSetModRole(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	guildID, _, err := requireAdmin(e)
	if err != nil {
		return err
	}
	roleID, err := optionRoleID(data)
	if err != nil {
		slog.Warn("could not read the role option", "guild_id", guildID, slog.Any("error", err))
		return ephemeralReply(e, configBadRoleMessage)
	}
	if err := SetModRole(e.Ctx, guildID, roleID); err != nil {
		slog.Warn("could not set the mod role", "guild_id", guildID, "role_id", roleID, slog.Any("error", err))
		return ephemeralReply(e, configWriteFailed)
	}
	return ephemeralReply(e, "The mod role is now "+roleMention(roleID)+".")
}

func handleConfigClearModRole(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	guildID, _, err := requireAdmin(e)
	if err != nil {
		return err
	}
	removed, err := ClearModRole(e.Ctx, guildID)
	if err != nil {
		slog.Warn("could not clear the mod role", "guild_id", guildID, slog.Any("error", err))
		return ephemeralReply(e, configWriteFailed)
	}
	if !removed {
		return ephemeralReply(e, configNoRoleMessage)
	}
	return ephemeralReply(e, "Cleared the mod role.")
}

// requireAdmin resolves an interaction to the guild it happened in and to that
// guild's settings, which deciding who may read them needed anyway. It replies
// with the reason and returns an error when the member invoking it may not.
//
// Red special-cases its owner for the config command: the guild and bot owner
// run it regardless of any role, so a misconfiguration can never strand them.
// That exemption, the lockout hook, is applied before the gate so a document
// that will not read cannot lock the owner out, and is scoped to config here
// rather than a universal bypass. The gate on PrivilegeGuildOwner then denies
// everyone else: only the guild and bot owner clear it, and those are the owners
// the hook already allowed, so a non-owner is denied.
func requireAdmin(e *handler.CommandEvent) (string, *GuildSettings, error) {
	guild, member := e.GuildID(), e.Member()
	if guild == nil || member == nil {
		return "", nil, ephemeralReply(e, configGuildOnlyMessage)
	}
	guildID := guild.String()
	cacheGuild, inGuild := e.Guild()
	var ownerGuild *discord.Guild
	if inGuild {
		ownerGuild = &cacheGuild.Guild
	}
	settings, err := Get[GuildSettings](e.Ctx, GuildScopeKey, guildID)
	if err != nil {
		slog.Warn("could not read guild settings", "guild_id", guildID, slog.Any("error", err))
		return "", nil, ephemeralReply(e, configReadFailed)
	}
	if isOwner(ownerGuild, member) {
		return guildID, &settings, nil
	}
	ok, err := Allowed(e.Ctx, guildID, ownerGuild, member, PrivilegeGuildOwner)
	if err != nil || !ok {
		slog.Warn("could not resolve the config gate", "guild_id", guildID, slog.Any("error", err))
		return "", nil, ephemeralReply(e, configDeniedMessage)
	}
	return guildID, &settings, nil
}

// optionRoleID returns the role the role option named.
//
// Discord's application commands have no repeated option: an option is supplied
// at most once per invocation, so the role arrives as one bare snowflake string
// rather than an array. Configuring several roles means invoking the subcommand
// once per role.
func optionRoleID(data discord.SlashCommandInteractionData) (string, error) {
	role, ok := data.OptRole(rolesOption)
	if !ok {
		return "", fmt.Errorf("option %q did not name a role", rolesOption)
	}
	if role.ID == 0 {
		return "", fmt.Errorf("option %q named the null role", rolesOption)
	}
	return role.ID.String(), nil
}

func roleMention(roleID string) string {
	return "<@&" + roleID + ">"
}

// ephemeralReply answers privately. What /config shows is the server's own
// settings, and who may read it is not something the channel should decide.
func ephemeralReply(e *handler.CommandEvent, content string) error {
	return e.CreateMessage(discord.MessageCreate{
		Content: truncate(content, messageContentLimit),
		Flags:   discord.MessageFlagEphemeral,
	})
}

func ephemeralEmbed(e *handler.CommandEvent, embed discord.Embed) error {
	return e.CreateMessage(discord.MessageCreate{
		Embeds: []discord.Embed{embed},
		Flags:  discord.MessageFlagEphemeral,
	})
}

// truncate cuts text to limit characters, saying so rather than sending
// something Discord rejects.
func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	if limit <= len(truncationNotice) {
		return text[:limit]
	}
	return text[:limit-len(truncationNotice)] + truncationNotice
}

const truncationNotice = "\n…"
