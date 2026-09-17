package sqlstore_test

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/bakhod1r/sessionx"
	"github.com/bakhod1r/sessionx/sqlstore"
	"github.com/bakhod1r/sessionx/storetest"
)

func TestConformance(t *testing.T) {
	storetest.Run(t, "sqlstore", func(t *testing.T) sessionx.Store {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		// One connection: an in-memory SQLite database is per-connection,
		// and a pool would give each query its own empty database.
		db.SetMaxOpenConns(1)

		if _, err := db.Exec(sqlstore.SchemaSQLite); err != nil {
			t.Fatalf("schema: %v", err)
		}
		return sqlstore.New(db, sqlstore.Options{})
	}, storetest.Capabilities{Enumerates: true})
}
