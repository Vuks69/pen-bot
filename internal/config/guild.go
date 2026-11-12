package config

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/disgoorg/disgo/discord"

	"github.com/Neon-Genesis-Linux/pen-bot/internal/db"
)

const (
	// GuildScopeKey names the guild-wide settings scope. A module checking who may
	// run a command reads it with Get[GuildSettings], so it is exported rather than
	// left for each caller to spell out.
	GuildScopeKey = "guild"
	// guildModule is the module name /config view groups this scope under.
	guildModule = "core"
)

// PrivilegeLevel is an ordered ladder of who may run a command, the shape Red's
// permissions cog gives the bot. Each level implies every one before it, so a
// member passes a command held on a level by resolving to that level or higher;
// the ladder is read lowest to highest and a comparison with >= enforces the
// implication.
type PrivilegeLevel int

const (
	// PrivilegeNone is what any member without a role or the owner holds, the floor
	// below every gated command.
	PrivilegeNone PrivilegeLevel = iota
	// PrivilegeModerator is a member who holds the configured mod role, which lets
	// them run commands that act on other members.
	PrivilegeModerator
	// PrivilegeAdmin is a member who holds the configured admin role, or who already
	// has the Administrator permission and can edit the guild's settings, so it is
	// granted by either.
	PrivilegeAdmin
	// PrivilegeGuildOwner is the guild's owner, the highest level one guild grants,
	// so it passes every command held on a lower level. The config command keeps a
	// separate, config-only owner bypass, where a misconfiguration must never
	// strand the owner.
	PrivilegeGuildOwner
	// PrivilegeBotOwner is someone the bot is configured to own, above the guild.
	PrivilegeBotOwner
)

// GuildSettings is the guild-wide scope, stored as the guild's whole settings
// document. Every other scope sits beside it in the framework's registry rather
// than inside it, so a guild that configured nothing still reads back two unset
// roles.
type GuildSettings struct {
	// AdminRole is the role a guild treats as admin, and therefore who may run
	// commands gated on PrivilegeAdmin. A nil value is meaningful, not a mistake:
	// no role grants admin by role until the guild sets one, and the Administrator
	// permission still grants it, so an unset admin role does not lock admin out.
	AdminRole *string `json:"admin_role,omitempty"`
	// ModRole is the role a guild treats as mod, and therefore who may run commands
	// gated on PrivilegeModerator. It is granted only by the configured role, with
	// no permission-flag fallback, unlike admin.
	ModRole *string `json:"mod_role,omitempty"`
}

// resolveLevel answers how much of the ladder a member holds in one guild,
// resolved from the same cached data a command reads here: the bot owner list,
// the guild, and the member. It resolves to the highest level the guild grants,
// which by the ladder's implication also passes any lower gate, from a bot owner
// down through the guild owner, an admin, and a mod to nothing. A nil guild
// holds no owner, so a member in a DM resolves to nothing, and a nil member
// does either.
//
// Admin is a mix, granted by the configured admin role OR the Administrator
// permission, so a member who can edit the guild's settings is an admin
// regardless of a role. Mod is strict, granted only by the configured mod role,
// so the permission flag is not enough for a mod.
func (s GuildSettings) resolveLevel(guild *discord.Guild, member *discord.ResolvedMember) PrivilegeLevel {
	if member == nil || guild == nil {
		return PrivilegeNone
	}
	if _, isBotOwner := BotOwners()[member.User.ID]; isBotOwner {
		return PrivilegeBotOwner
	}
	if guild.OwnerID == member.User.ID {
		return PrivilegeGuildOwner
	}
	if member.Permissions.Has(discord.PermissionAdministrator) {
		return PrivilegeAdmin
	}
	if s.adminRole() == "" && s.modRole() == "" {
		return PrivilegeNone
	}
	for _, roleID := range member.RoleIDs {
		if roleID.String() == s.adminRole() {
			return PrivilegeAdmin
		}
		if roleID.String() == s.modRole() {
			return PrivilegeModerator
		}
	}
	return PrivilegeNone
}

// adminRole and modRole dereference the optional role ids to a bare string that
// never matches a role id, so a guild with no admin or mod role set matches
// nothing rather than every role.
func (s GuildSettings) adminRole() string {
	if s.AdminRole == nil {
		return ""
	}
	return *s.AdminRole
}

func (s GuildSettings) modRole() string {
	if s.ModRole == nil {
		return ""
	}
	return *s.ModRole
}

// isOwner reports whether a member is exempt from every config gate: the guild
// owner, recognised from the cached guild's OwnerID with no REST call, or a bot
// owner. It is the one guarantee Red makes for its config command, so a
// misconfiguration can never strand the owner, and it is scoped to config
// rather than applied to every command.
func isOwner(guild *discord.Guild, member *discord.ResolvedMember) bool {
	if member == nil {
		return false
	}
	if _, isBotOwner := BotOwners()[member.User.ID]; isBotOwner {
		return true
	}
	return guild != nil && guild.OwnerID == member.User.ID
}

// Allowed reports whether a member passes a command's gate on the required
// privilege level, the one-line gate a command runs, loading the guild's settings
// for the admin and mod roles first. A read that cannot complete reports an
// error, which a caller should treat as not allowed.
//
//	if ok, _ := config.Allowed(e.Ctx, guildID, e.Guild(), e.Member(), config.PrivilegeModerator); !ok {
//		return deny
//	}
//
// It resolves every level from cached data with no REST call: the guild owner
// from the cached guild's OwnerID, an admin from the configured role or the
// Administrator permission, a mod from the configured role. A guild that could
// not be read is treated as no one holding the ladder, so a command denies rather
// than failing on a read that cannot complete.
func Allowed(ctx context.Context, guildID string, guild *discord.Guild, member *discord.ResolvedMember, required PrivilegeLevel) (bool, error) {
	settings, err := Get[GuildSettings](ctx, GuildScopeKey, guildID)
	if err != nil {
		return false, err
	}
	return settings.resolveLevel(guild, member) >= required, nil
}

// SetAdminRole records that a guild treats a role as its admin role, writing the
// single json path atomically so two admins configuring the admin and mod roles
// at the same time cannot discard each other's change, then drops the cache so
// the change is seen.
func SetAdminRole(ctx context.Context, guildID, roleID string) error {
	return setGuildRole(ctx, guildID, "$.admin_role", roleID)
}

// SetModRole records that a guild treats a role as its mod role, writing the
// single json path atomically, then drops the cache so the change is seen.
func SetModRole(ctx context.Context, guildID, roleID string) error {
	return setGuildRole(ctx, guildID, "$.mod_role", roleID)
}

// ClearAdminRole drops a guild's admin role, reporting whether a guild held one.
func ClearAdminRole(ctx context.Context, guildID string) (bool, error) {
	return removeGuildRole(ctx, guildID, "$.admin_role")
}

// ClearModRole drops a guild's mod role, reporting whether a guild held one.
func ClearModRole(ctx context.Context, guildID string) (bool, error) {
	return removeGuildRole(ctx, guildID, "$.mod_role")
}

// setGuildRole writes roleID at jsonPath, reporting errNoDatabase when the bot
// holds no database handle yet.
func setGuildRole(ctx context.Context, guildID, jsonPath, roleID string) error {
	database := db.GlobalDB()
	if database == nil {
		return errNoDatabase
	}
	if err := db.SetGuildSetting(ctx, database, guildID, jsonPath, roleID); err != nil {
		return err
	}
	db.DeleteCacheEntry(cacheKey(guildID, GuildScopeKey))
	return nil
}

// removeGuildRole drops jsonPath, reporting errNoDatabase when the bot holds no
// database handle yet, and only dropping the cache when a role was actually
// removed.
func removeGuildRole(ctx context.Context, guildID, jsonPath string) (bool, error) {
	database := db.GlobalDB()
	if database == nil {
		return false, errNoDatabase
	}
	removed, err := db.RemoveGuildSetting(ctx, database, guildID, jsonPath)
	if err != nil || !removed {
		return removed, err
	}
	db.DeleteCacheEntry(cacheKey(guildID, GuildScopeKey))
	return true, nil
}

// registerGuildScope registers the guild-wide settings with the framework.
func registerGuildScope() error {
	return RegisterScope(ScopeDesc[GuildSettings]{
		Key:     GuildScopeKey,
		Module:  guildModule,
		Summary: "The role that acts as admin or mod, if any.",
		// No admin or mod role is set by default, so a guild that configured
		// nothing reads both back unset, and admin then comes only from the
		// Administrator permission. There is no validation for either role.
		Defaults: GuildSettings{AdminRole: nil, ModRole: nil},
		Validate: nil,
	}, loadGuildDocument, saveGuildDocument, resetGuildDocument)
}

// loadGuildDocument returns the guild's stored settings document, reporting
// errNoDatabase when the bot holds no database handle yet.
func loadGuildDocument(ctx context.Context, guildID string) (json.RawMessage, error) {
	database := db.GlobalDB()
	if database == nil {
		return nil, errNoDatabase
	}
	return db.GetGuildDocument(ctx, database, guildID)
}

// saveGuildDocument replaces the guild's stored settings document.
func saveGuildDocument(ctx context.Context, guildID string, raw json.RawMessage) error {
	database := db.GlobalDB()
	if database == nil {
		return errNoDatabase
	}
	return db.SetGuildDocument(ctx, database, guildID, string(raw))
}

// resetGuildDocument always fails: the guild scope mixes the built-ins with the
// guild's own roles, so it cannot return to its defaults in one step. Callers
// that want to clear it must remove roles one at a time with /config admin-role
// clear and /config mod-role clear.
func resetGuildDocument(_ context.Context, _ string) (bool, error) {
	return false, fmt.Errorf("%w, remove roles one at a time with /config admin-role clear and /config mod-role clear",
		ErrScopeNotResettable)
}
