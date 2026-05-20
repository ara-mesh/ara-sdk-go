package ara_test

import (
	"context"
	"testing"

	ara "github.com/ara-mesh/ara-sdk-go"
)

var testMigrations = []ara.Migration{
	{
		Version:     1,
		Description: "items table",
		SQL:         `CREATE TABLE items (id TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '') STRICT;`,
		Sync:        []string{"items"},
	},
}

func TestOpenClose(t *testing.T) {
	ctx := context.Background()
	node, err := ara.Open(ctx, ara.Config{
		Path:       ":memory:",
		Migrations: testMigrations,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := node.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExecQuery(t *testing.T) {
	ctx := context.Background()
	node, err := ara.Open(ctx, ara.Config{Path: ":memory:", Migrations: testMigrations})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Close()

	if err := node.Exec(ctx, `INSERT INTO items (id, name) VALUES (?, ?)`, "a1", "hello"); err != nil {
		t.Fatal(err)
	}

	rows, err := node.Query(ctx, `SELECT id, name FROM items`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0]["name"] != "hello" {
		t.Fatalf("expected name=hello, got %v", rows[0]["name"])
	}
}

func TestQueryRow(t *testing.T) {
	ctx := context.Background()
	node, err := ara.Open(ctx, ara.Config{Path: ":memory:", Migrations: testMigrations})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Close()

	node.Exec(ctx, `INSERT INTO items (id, name) VALUES (?, ?)`, "b1", "world") //nolint:errcheck

	row := node.QueryRow(ctx, `SELECT COUNT(*) as n FROM items`)
	if row.Err() != nil {
		t.Fatal(row.Err())
	}
	var count int
	if err := row.Get("n", &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected count=1, got %d", count)
	}
}

func TestQueryRowNoRows(t *testing.T) {
	ctx := context.Background()
	node, err := ara.Open(ctx, ara.Config{Path: ":memory:", Migrations: testMigrations})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Close()

	row := node.QueryRow(ctx, `SELECT id FROM items WHERE id = 'nope'`)
	if row.Err() != ara.ErrNoRows {
		t.Fatalf("expected ErrNoRows, got %v", row.Err())
	}
}

func TestNodeID(t *testing.T) {
	ctx := context.Background()
	node, err := ara.Open(ctx, ara.Config{Path: ":memory:", Migrations: testMigrations})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Close()

	id := node.NodeID()
	if len(id) != 36 { // UUID format: 8-4-4-4-12
		t.Fatalf("expected UUID, got %q", id)
	}
}

func TestPeers(t *testing.T) {
	ctx := context.Background()
	node, err := ara.Open(ctx, ara.Config{Path: ":memory:", Migrations: testMigrations})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Close()

	peers, err := node.Peers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// no transport added, so no peers
	if len(peers) != 0 {
		t.Fatalf("expected 0 peers, got %d", len(peers))
	}
}
