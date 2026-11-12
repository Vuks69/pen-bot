package db_test

import (
	"context"
	"testing"

	"github.com/Neon-Genesis-Linux/pen-bot/internal/db"
)

// roleNineAdmin is the guild document with a single admin role, compared often
// enough that goconst wants it pulled out of the assertions.
const roleNineAdmin = `{"admin_role":"9"}`

// These exercise the settings SQL against the real SQLite driver, so the
// SQLite JSON and date functions these queries rely on are pinned here. They
// are in package db_test, which is the only package that has tests and needs
// no server.

func TestSetGuildDocumentRoundTripsAndUpserts(t *testing.T) {
	ctx := context.Background()
	database := testDB(t)
	const guild = "g1"

	if raw, err := db.GetGuildDocument(ctx, database, guild); err != nil || raw != nil {
		t.Fatalf("missing guild document = %q, %v: want nil", raw, err)
	}

	if err := db.SetGuildDocument(ctx, database, guild, `{"admin_role":"9"}`); err != nil {
		t.Fatalf("set: %v", err)
	}
	raw, err := db.GetGuildDocument(ctx, database, guild)
	if err != nil || string(raw) != `{"admin_role":"9"}` {
		t.Fatalf("round trip = %q, %v: want the stored document", raw, err)
	}

	if err := db.SetGuildDocument(ctx, database, guild, `{"mod_role":"10"}`); err != nil {
		t.Fatalf("upsert set: %v", err)
	}
	raw, err = db.GetGuildDocument(ctx, database, guild)
	if err != nil || string(raw) != `{"mod_role":"10"}` {
		t.Fatalf("after upsert = %q, %v: want the replaced document", raw, err)
	}
}

// TestSetGuildSettingWritesEachPathIndependently checks the path-based write:
// two different roles are recorded under their own paths, and a later write to
// one path leaves the other intact rather than replacing the whole document.
func TestSetGuildSettingWritesEachPathIndependently(t *testing.T) {
	ctx := context.Background()
	database := testDB(t)
	const guild = "g1"

	if err := db.SetGuildSetting(ctx, database, guild, "$.admin_role", "9"); err != nil {
		t.Fatalf("set first role: %v", err)
	}
	raw, err := db.GetGuildDocument(ctx, database, guild)
	if err != nil || string(raw) != roleNineAdmin {
		t.Fatalf("after first role = %q, %v", raw, err)
	}

	// A second role under a different path must not overwrite the first.
	if err := db.SetGuildSetting(ctx, database, guild, "$.mod_role", "10"); err != nil {
		t.Fatalf("set second role: %v", err)
	}
	raw, err = db.GetGuildDocument(ctx, database, guild)
	if err != nil || string(raw) != `{"admin_role":"9","mod_role":"10"}` {
		t.Fatalf("after second role = %q, %v: want both roles", raw, err)
	}
}

// TestRemoveGuildSettingDropsThePathAndReportsIt checks the path-based removal:
// one role is dropped while the other is left, and a path the guild never
// configured touches no row.
func TestRemoveGuildSettingDropsThePathAndReportsIt(t *testing.T) {
	ctx := context.Background()
	database := testDB(t)
	const guild = "g1"

	if err := db.SetGuildSetting(ctx, database, guild, "$.admin_role", "9"); err != nil {
		t.Fatalf("set role: %v", err)
	}
	if err := db.SetGuildSetting(ctx, database, guild, "$.mod_role", "10"); err != nil {
		t.Fatalf("set role: %v", err)
	}

	removed, err := db.RemoveGuildSetting(ctx, database, guild, "$.mod_role")
	if err != nil || !removed {
		t.Fatalf("remove = %v, %v: want true", removed, err)
	}
	raw, err := db.GetGuildDocument(ctx, database, guild)
	if err != nil || string(raw) != roleNineAdmin {
		t.Fatalf("after remove = %q, %v: want only the remaining role", raw, err)
	}

	// Removing a path the guild never configured touches no row.
	if removed, err := db.RemoveGuildSetting(ctx, database, guild, "$.unknown"); err != nil || removed {
		t.Fatalf("remove unknown = %v, %v: want false", removed, err)
	}
	raw, err = db.GetGuildDocument(ctx, database, guild)
	if err != nil || string(raw) != roleNineAdmin {
		t.Fatalf("document changed on an unguarded path: %q, %v", raw, err)
	}
}
