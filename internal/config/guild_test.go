package config

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

const (
	adminRoleID  = "1000000000000000001"
	modRoleID    = "1000000000000000002"
	memberRoleID = "1000000000000000003"
	ownerID      = "1000000000000000004"
	botOwnerID   = "1000000000000000005"
	strangerID   = "1000000000000000009"
)

// snowflakeID parses a compile-time literal, so the role and owner ids in this
// file never need a test-aware parse.
func snowflakeID(id string) snowflake.ID {
	parsed, err := snowflake.Parse(id)
	if err != nil {
		panic(id)
	}
	return parsed
}

// stringPtr is the one-line address of a string, so a test can set a role on a
// GuildSettings without a named variable.
func stringPtr(value string) *string {
	return &value
}

// member resolves a user and an optional role list into a member, the shape
// resolveLevel reads.
func member(userSnowflake string, roleIDs ...string) *discord.ResolvedMember {
	user := snowflakeID(userSnowflake)
	ids := make([]snowflake.ID, 0, len(roleIDs))
	for _, id := range roleIDs {
		ids = append(ids, snowflakeID(id))
	}
	return &discord.ResolvedMember{Member: discord.Member{User: discord.User{ID: user}, RoleIDs: ids}}
}

func guildWithOwner(ownerSnowflake string) *discord.Guild {
	return &discord.Guild{OwnerID: snowflakeID(ownerSnowflake)}
}

// adminMember resolves to admin purely by the Administrator permission.
func adminMember(userSnowflake string) *discord.ResolvedMember {
	return &discord.ResolvedMember{
		Member:      discord.Member{User: discord.User{ID: snowflakeID(userSnowflake)}},
		Permissions: discord.PermissionAdministrator,
	}
}

// TestResolveLevelOrdersTheLadder checks the order a member resolves in: a bot
// owner first, then the guild owner, then an admin, a mod, then nothing. Each
// rung is the highest level the member holds, and a level never implies a higher
// one.
func TestResolveLevelOrdersTheLadder(t *testing.T) {
	t.Setenv(botOwnerEnv, botOwnerID)
	settings := GuildSettings{AdminRole: stringPtr(adminRoleID), ModRole: stringPtr(modRoleID)}
	owner := guildWithOwner(ownerID)

	tests := []struct {
		name     string
		settings GuildSettings
		guild    *discord.Guild
		member   *discord.ResolvedMember
		want     PrivilegeLevel
	}{
		{
			name:     "a bot owner holds the top of the ladder",
			settings: settings,
			guild:    owner,
			member:   member(botOwnerID),
			want:     PrivilegeBotOwner,
		},
		{
			name:     "the guild owner holds the guild's top",
			settings: settings,
			guild:    owner,
			member:   member(ownerID),
			want:     PrivilegeGuildOwner,
		},
		{
			name:     "the Administrator permission grants admin by a mix",
			settings: settings,
			guild:    owner,
			member:   adminMember(strangerID),
			want:     PrivilegeAdmin,
		},
		{
			name:     "the configured admin role grants admin without the permission",
			settings: settings,
			guild:    owner,
			member:   member(adminRoleID, adminRoleID),
			want:     PrivilegeAdmin,
		},
		{
			name:     "the configured mod role grants mod, and nothing more",
			settings: settings,
			guild:    owner,
			member:   member(modRoleID, modRoleID),
			want:     PrivilegeModerator,
		},
		{
			name:     "an admin role does not reach moderator",
			settings: settings,
			guild:    owner,
			member:   member(adminRoleID, adminRoleID),
			want:     PrivilegeAdmin,
		},
		{
			name:     "a member with no roles holds nothing",
			settings: settings,
			guild:    owner,
			member:   member(memberRoleID),
			want:     PrivilegeNone,
		},
		{
			name:     "the permission does not reach moderator",
			settings: GuildSettings{ModRole: stringPtr(modRoleID)},
			guild:    owner,
			member:   adminMember(strangerID),
			want:     PrivilegeAdmin,
		},
		{
			name:     "no roles configured and no permission yields nothing",
			settings: GuildSettings{},
			guild:    owner,
			member:   member(strangerID),
			want:     PrivilegeNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.settings.resolveLevel(tc.guild, tc.member); got != tc.want {
				t.Errorf("resolveLevel = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestResolveLevelStopsOnNilChecks checks that a missing guild or member holds
// nothing, so a nil guild (a DM) and a nil member resolve to the floor rather
// than panic.
func TestResolveLevelStopsOnNilChecks(t *testing.T) {
	settings := GuildSettings{AdminRole: stringPtr(adminRoleID), ModRole: stringPtr(modRoleID)}

	if got := settings.resolveLevel(guildWithOwner(ownerID), nil); got != PrivilegeNone {
		t.Errorf("resolveLevel(nil member) = %v, want none", got)
	}
	if got := settings.resolveLevel(nil, member(strangerID)); got != PrivilegeNone {
		t.Errorf("resolveLevel(nil guild) = %v, want none", got)
	}
}

// TestResolveLevelMatchesTheLevelGate checks that a member passes a gate held on
// a level by resolving to that level or higher, which is the implication the
// >= comparison enforces.
func TestResolveLevelMatchesTheLevelGate(t *testing.T) {
	settings := GuildSettings{AdminRole: stringPtr(adminRoleID), ModRole: stringPtr(modRoleID)}
	owner := guildWithOwner(ownerID)

	memberAdmin := member(adminRoleID, adminRoleID)
	if got := settings.resolveLevel(owner, memberAdmin); got < PrivilegeModerator {
		t.Errorf("resolveLevel(admin) = %v, want >= moderator", got)
	}
	if got := settings.resolveLevel(owner, member(modRoleID, modRoleID)); got < PrivilegeModerator {
		t.Errorf("resolveLevel(mod) = %v, want >= moderator", got)
	}
	if got := settings.resolveLevel(owner, member(memberRoleID)); got >= PrivilegeModerator {
		t.Errorf("resolveLevel(bare) = %v, want < moderator", got)
	}
}

// TestIsOwnerChecksTheConfigBypass checks the owner bypass the config command
// relies on: the guild owner and a bot owner are owners, and nobody else is. A
// nil member is not an owner, so the command stays closed to them.
func TestIsOwnerChecksTheConfigBypass(t *testing.T) {
	t.Setenv(botOwnerEnv, botOwnerID)
	owner := guildWithOwner(ownerID)

	if !isOwner(owner, member(ownerID)) {
		t.Error("isOwner(guild owner) = false, want true")
	}
	if !isOwner(nil, member(botOwnerID)) {
		t.Error("isOwner(bot owner, nil guild) = false, want true")
	}
	if isOwner(owner, member(strangerID)) {
		t.Error("isOwner(stranger) = true, want false")
	}
	if isOwner(owner, nil) {
		t.Error("isOwner(nil member) = true, want false")
	}
}
