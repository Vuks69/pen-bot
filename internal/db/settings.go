package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// GetGuildDocument returns the guild's settings document, or nil when the guild
// has never been configured.
func GetGuildDocument(ctx context.Context, db *DB, guildID string) ([]byte, error) {
	if guildID == "" {
		return nil, fmt.Errorf("guildID required")
	}
	var raw []byte
	err := db.QueryRowContext(ctx, `
	SELECT settings_json FROM guilds WHERE guild_id = $1
	`, guildID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// SetGuildDocument replaces the guild's settings document outright, creating the
// guild's row when it does not exist yet.
//
// It is the coarse counterpart to SetGuildSetting: a caller that read the
// document before writing it discards every change made since.
func SetGuildDocument(ctx context.Context, db *DB, guildID, documentJSON string) error {
	if guildID == "" {
		return fmt.Errorf("guildID required")
	}
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
	INSERT INTO guilds (guild_id, settings_json, created_at, updated_at)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (guild_id) DO UPDATE SET
		settings_json = EXCLUDED.settings_json,
		updated_at = $4
	`, guildID, documentJSON, now, now)
	return err
}

// SetGuildSetting writes value at jsonPath in the guild's settings document,
// creating the row when absent, so two admins configuring different roles at the
// same time cannot discard each other's change, and a guild whose document lacks
// the key yet gets one created.
//
// The path is bound as a parameter, so a role id taken from Discord can neither
// rewrite another key nor escape the settings object. json_set adds or replaces
// the leaf in one statement, whether the row is new or existing, so the write is
// atomic.
func SetGuildSetting(ctx context.Context, db *DB, guildID, jsonPath string, value any) error {
	if guildID == "" {
		return fmt.Errorf("guildID required")
	}
	if jsonPath == "" {
		return fmt.Errorf("jsonPath required")
	}
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
	INSERT INTO guilds (guild_id, settings_json, created_at, updated_at)
	VALUES ($1, json_set('{}', $4, $5), $2, $3)
	ON CONFLICT (guild_id) DO UPDATE SET
		settings_json = json_set(guilds.settings_json, $4, $5),
		updated_at = $3
	`, guildID, now, now, jsonPath, value)
	return err
}

// RemoveGuildSetting drops jsonPath from the guild's document, reporting whether
// the guild held one. The path is guarded so that a role the guild never
// configured updates no row: an unguarded rewrite reports the row as affected
// even when the document comes out identical.
func RemoveGuildSetting(ctx context.Context, db *DB, guildID, jsonPath string) (bool, error) {
	if guildID == "" {
		return false, fmt.Errorf("guildID required")
	}
	if jsonPath == "" {
		return false, fmt.Errorf("jsonPath required")
	}
	now := time.Now().UTC()
	result, err := db.ExecContext(ctx, `
	UPDATE guilds SET settings_json = json_remove(settings_json, $2), updated_at = $3
	WHERE guild_id = $1 AND json_extract(settings_json, $2) IS NOT NULL
	`, guildID, jsonPath, now)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}
