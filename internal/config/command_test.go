package config

import (
	"encoding/json"
	"testing"

	"github.com/disgoorg/disgo/discord"
)

// configSetAdminRoleInteraction is a real /config set-admin-role payload,
// trimmed to the interaction data. It records the shape Discord actually sends:
// the role option arrives as one bare snowflake string under type 8, because
// Discord's application commands have no repeated option.
const configSetAdminRoleInteraction = `{
	"id": "1",
	"type": 2,
	"application_id": "1",
	"token": "token",
	"version": 1,
	"guild_id": "701021825563361280",
	"channel_id": "1",
	"context": 0,
	"member": {
		"user": {"id": "1", "username": "owner"},
		"roles": []
	},
	"data": {
		"id": "2",
		"name": "config",
		"type": 1,
		"options": [
			{
				"type": 1,
				"name": "set-admin-role",
				"options": [
					{"type": 8, "name": "role", "value": "1555995781356458065"}
				]
			}
		],
		"resolved": {
			"roles": {
				"1555995781356458065": {
					"id": "1555995781356458065",
					"guild_id": "701021825563361280",
					"name": "admin",
					"position": 2,
					"permissions": "0",
					"mentionable": false,
					"hoist": false,
					"managed": false
				}
			}
		}
	}
}`

func TestOptionRoleIDReadsTheSingleRoleDiscordSends(t *testing.T) {
	var interaction discord.ApplicationCommandInteraction
	if err := json.Unmarshal([]byte(configSetAdminRoleInteraction), &interaction); err != nil {
		t.Fatalf("decoding the interaction failed: %v", err)
	}
	data, ok := interaction.Data.(discord.SlashCommandInteractionData)
	if !ok {
		t.Fatalf("interaction data is %T, want discord.SlashCommandInteractionData", interaction.Data)
	}

	roleID, err := optionRoleID(data)
	if err != nil {
		t.Fatalf("optionRoleID failed: %v", err)
	}
	if roleID != "1555995781356458065" {
		t.Errorf("optionRoleID = %q, want %q", roleID, "1555995781356458065")
	}
}

func TestOptionRoleIDReportsAnAbsentOrUnusableRole(t *testing.T) {
	var interaction discord.ApplicationCommandInteraction
	if err := json.Unmarshal([]byte(`{"id":"1","type":2,"data":{"id":"2","name":"config","type":1}}`), &interaction); err != nil {
		t.Fatalf("decoding the interaction failed: %v", err)
	}

	data, ok := interaction.Data.(discord.SlashCommandInteractionData)
	if !ok {
		t.Fatalf("interaction data is %T, want discord.SlashCommandInteractionData", interaction.Data)
	}
	if roleID, err := optionRoleID(data); err == nil {
		t.Errorf("optionRoleID = %q, want an error when the option is absent", roleID)
	}

	if err := json.Unmarshal([]byte(`{"id":"1","type":2,"data":{"id":"2","name":"config","type":1,
		"options":[{"type":8,"name":"role","value":"0"}]}}`), &interaction); err != nil {
		t.Fatalf("decoding the interaction failed: %v", err)
	}
	data, ok = interaction.Data.(discord.SlashCommandInteractionData)
	if !ok {
		t.Fatalf("interaction data is %T, want discord.SlashCommandInteractionData", interaction.Data)
	}
	if roleID, err := optionRoleID(data); err == nil {
		t.Errorf("optionRoleID = %q, want an error for the null role", roleID)
	}
}

// TestConfigCommandCreateDescribesEverySubcommand checks that /config exposes the
// four role subcommands and that only the two set subcommands carry the shared
// role parameter, which the handler then reads.
func TestConfigCommandCreateDescribesEverySubcommand(t *testing.T) {
	create := configCommandCreate()

	byName := map[string]discord.ApplicationCommandOptionSubCommand{}
	for _, opt := range create.Options {
		sub, ok := opt.(discord.ApplicationCommandOptionSubCommand)
		if !ok {
			t.Fatalf("option %T is not a subcommand", opt)
		}
		byName[sub.Name] = sub
	}

	for _, want := range []string{
		viewSubcommand, rolesSubcommand,
		setAdminRoleCommand, clearAdminRoleCommand,
		setModRoleCommand, clearModRoleCommand,
	} {
		if _, ok := byName[want]; !ok {
			t.Errorf("missing subcommand %q", want)
		}
	}

	// The set subcommands take the role parameter, the clears do not: Discord
	// sends no option for a cleared subcommand.
	for _, set := range []string{setAdminRoleCommand, setModRoleCommand} {
		if len(byName[set].Options) != 1 {
			t.Errorf("%q options = %d, want the single role parameter", set, len(byName[set].Options))
		}
	}
	for _, clear := range []string{clearAdminRoleCommand, clearModRoleCommand} {
		if len(byName[clear].Options) != 0 {
			t.Errorf("%q options = %d, want none", clear, len(byName[clear].Options))
		}
	}
}

// TestRoleOptionIsTheSharedParameter checks the shared role parameter the set
// subcommands reference: named and required, so a set subcommand always carries
// it.
func TestRoleOptionIsTheSharedParameter(t *testing.T) {
	if roleOption.Name != rolesOption {
		t.Errorf("roleOption.Name = %q, want %q", roleOption.Name, rolesOption)
	}
	if !roleOption.Required {
		t.Error("roleOption.Required = false, want true")
	}
}

// TestAddRoleFieldRendersAMentionOrPlaceholder checks /config roles shows a role
// mention when a role is set, and the placeholder when the slot is empty.
func TestAddRoleFieldRendersAMentionOrPlaceholder(t *testing.T) {
	role := "222"
	embed := addRoleField(discord.NewEmbed(), "Admin", &role)
	if len(embed.Fields) != 1 || embed.Fields[0].Name != "Admin" || embed.Fields[0].Value != "<@&222>" {
		t.Errorf("addRoleField with a role = %+v, want the mention", embed.Fields)
	}

	embed = addRoleField(discord.NewEmbed(), "Mod", nil)
	if len(embed.Fields) != 1 || embed.Fields[0].Value != noRoleConfigured {
		t.Errorf("addRoleField with nil = %+v, want the placeholder", embed.Fields)
	}
}

// TestConfiguredCountsTheSetRoles checks how many of the two role slots a guild
// has filled, for the footer /config view shows.
func TestConfiguredCountsTheSetRoles(t *testing.T) {
	admin := "9"
	mod := "10"
	if got := configuredCount(&GuildSettings{AdminRole: &admin, ModRole: &mod}); got != 2 {
		t.Errorf("configuredCount(both) = %d, want 2", got)
	}
	if got := configuredCount(&GuildSettings{AdminRole: &admin}); got != 1 {
		t.Errorf("configuredCount(one) = %d, want 1", got)
	}
	if got := configuredCount(&GuildSettings{}); got != 0 {
		t.Errorf("configuredCount(neither) = %d, want 0", got)
	}
}

// TestTruncateKeepsContentWithinTheDiscordLimit checks that a message over the
// limit is shortened rather than sent as something Discord rejects, and a short
// one passes through unchanged.
func TestTruncateKeepsContentWithinTheDiscordLimit(t *testing.T) {
	long := make([]byte, 100)
	for i := range long {
		long[i] = 'x'
	}
	if got := truncate(string(long), 10); len(got) > 10 {
		t.Errorf("truncate length = %d, want <= 10", len(got))
	}

	short := "ok"
	if got := truncate(short, messageContentLimit); got != short {
		t.Errorf("truncate(short) = %q, want it unchanged", got)
	}
}
