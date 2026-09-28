package sqlstore_test

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/bakhod1r/sessionx"
	"github.com/bakhod1r/sessionx/sqlstore"
	"github.com/bakhod1r/sessionx/storetest"
)

// TestConformancePostgres runs the suite against a real PostgreSQL when
// SESSIONX_PG_DSN is set (CI starts one as a service). Each subtest gets its
// own table, so they do not see each other's rows.
func TestConformancePostgres(t *testing.T) {
	dsn := os.Getenv("SESSIONX_PG_DSN")
	if dsn == "" {
		t.Skip("SESSIONX_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	n := 0
	storetest.Run(t, "postgres", func(t *testing.T) sessionx.Store {
		n++
		table := fmt.Sprintf("sessions_%d_%d", time.Now().UnixNano(), n)
		ddl := fmt.Sprintf(`CREATE TABLE %s (
    id TEXT PRIMARY KEY, user_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL,
    device JSONB NOT NULL DEFAULT '{}', client JSONB NOT NULL DEFAULT '{}',
    network JSONB NOT NULL DEFAULT '{}', data JSONB NOT NULL DEFAULT '{}',
    locale TEXT NOT NULL DEFAULT '', locale_source TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL, last_seen TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL, renewed_at TIMESTAMPTZ)`, table)
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("schema: %v", err)
		}
		t.Cleanup(func() { _, _ = db.Exec("DROP TABLE " + table) })
		return sqlstore.New(db, sqlstore.Options{Table: table, Placeholder: sqlstore.Dollar})
	}, storetest.Capabilities{Enumerates: true})
}
