// Package sqlstore is a sessionx Store over database/sql.
//
// It imports no driver and no third-party package: the caller opens the
// database with whichever driver they already use, and hands the *sql.DB
// over. That is what keeps this store usable on PostgreSQL, MySQL and
// SQLite without the library choosing for them.
package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bakhod1r/sessionx"
)

// SchemaPostgres is the DDL for PostgreSQL, matching schema.sql.
const SchemaPostgres = `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    device JSONB NOT NULL DEFAULT '{}',
    client JSONB NOT NULL DEFAULT '{}',
    network JSONB NOT NULL DEFAULT '{}',
    data JSONB NOT NULL DEFAULT '{}',
    locale TEXT NOT NULL DEFAULT '',
    locale_source TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    last_seen TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    renewed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions (user_id);
CREATE INDEX IF NOT EXISTS sessions_expires_idx ON sessions (expires_at);`

// SchemaSQLite is the DDL for SQLite, used by the tests and by anyone
// embedding the database.
const SchemaSQLite = `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    device TEXT NOT NULL DEFAULT '{}',
    client TEXT NOT NULL DEFAULT '{}',
    network TEXT NOT NULL DEFAULT '{}',
    data TEXT NOT NULL DEFAULT '{}',
    locale TEXT NOT NULL DEFAULT '',
    locale_source TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL,
    last_seen TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    renewed_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions (user_id);
CREATE INDEX IF NOT EXISTS sessions_expires_idx ON sessions (expires_at);`

// Placeholder is the parameter syntax the target database speaks.
type Placeholder int

const (
	// Question is the ? syntax of MySQL and SQLite. The default.
	Question Placeholder = iota

	// Dollar is the $1 syntax of PostgreSQL.
	Dollar
)

// Options configures the store. The zero value targets a table named
// "sessions" with ? placeholders.
type Options struct {
	// Table is the table name. Empty means "sessions".
	Table string

	// Placeholder is the parameter syntax. Set Dollar for PostgreSQL.
	Placeholder Placeholder
}

// Store persists sessions in a SQL table.
type Store struct {
	db    *sql.DB
	table string
	ph    Placeholder
}

// New returns a store over db.
func New(db *sql.DB, opts Options) *Store {
	table := opts.Table
	if table == "" {
		table = "sessions"
	}
	return &Store{db: db, table: table, ph: opts.Placeholder}
}

// arg renders the n-th placeholder, 1-based.
func (s *Store) arg(n int) string {
	if s.ph == Dollar {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// args renders n placeholders separated by commas.
func (s *Store) args(n int) string {
	out := make([]string, n)
	for i := range out {
		out[i] = s.arg(i + 1)
	}
	return strings.Join(out, ", ")
}

// Save upserts the session by primary key.
func (s *Store) Save(ctx context.Context, sess *sessionx.Session) error {
	device, err := json.Marshal(sess.Device)
	if err != nil {
		return err
	}
	client, err := json.Marshal(sess.Client)
	if err != nil {
		return err
	}
	network, err := json.Marshal(sess.Network)
	if err != nil {
		return err
	}
	data, err := json.Marshal(sess.Data)
	if err != nil {
		return err
	}

	var renewed any
	if !sess.RenewedAt.IsZero() {
		renewed = sess.RenewedAt.UTC()
	}

	q := fmt.Sprintf(`INSERT INTO %s
        (id, user_id, status, device, client, network, data, locale, locale_source,
         created_at, last_seen, expires_at, renewed_at)
        VALUES (%s)
        ON CONFLICT (id) DO UPDATE SET
            user_id = excluded.user_id,
            status = excluded.status,
            device = excluded.device,
            client = excluded.client,
            network = excluded.network,
            data = excluded.data,
            locale = excluded.locale,
            locale_source = excluded.locale_source,
            last_seen = excluded.last_seen,
            expires_at = excluded.expires_at,
            renewed_at = excluded.renewed_at`, s.table, s.args(13))

	_, err = s.db.ExecContext(ctx, q,
		sess.ID, sess.UserID, string(sess.Status),
		string(device), string(client), string(network), string(data),
		sess.Locale, sess.LocaleSource,
		sess.CreatedAt.UTC(), sess.LastSeen.UTC(), sess.ExpiresAt.UTC(), renewed,
	)
	return err
}

const columns = `id, user_id, status, device, client, network, data, locale, locale_source, ` +
	`created_at, last_seen, expires_at, renewed_at`

// Load reads one session, returning sessionx.ErrNotFound when the id is
// unknown.
func (s *Store) Load(ctx context.Context, id string) (*sessionx.Session, error) {
	q := fmt.Sprintf(`SELECT %s FROM %s WHERE id = %s`, columns, s.table, s.arg(1))

	sess, err := scan(s.db.QueryRowContext(ctx, q, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sessionx.ErrNotFound
	}
	return sess, err
}

// Delete removes one session. Removing an absent row is not an error.
func (s *Store) Delete(ctx context.Context, id string) error {
	q := fmt.Sprintf(`DELETE FROM %s WHERE id = %s`, s.table, s.arg(1))
	_, err := s.db.ExecContext(ctx, q, id)
	return err
}

// ListByUser returns every session belonging to userID.
func (s *Store) ListByUser(ctx context.Context, userID string) ([]*sessionx.Session, error) {
	q := fmt.Sprintf(`SELECT %s FROM %s WHERE user_id = %s`, columns, s.table, s.arg(1))

	rows, err := s.db.QueryContext(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []*sessionx.Session
	for rows.Next() {
		sess, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// DeleteByUser removes every session belonging to userID.
func (s *Store) DeleteByUser(ctx context.Context, userID string) (int, error) {
	q := fmt.Sprintf(`DELETE FROM %s WHERE user_id = %s`, s.table, s.arg(1))

	res, err := s.db.ExecContext(ctx, q, userID)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// GC removes every session whose deadline has passed.
func (s *Store) GC(ctx context.Context, now time.Time) (int, error) {
	q := fmt.Sprintf(`DELETE FROM %s WHERE expires_at < %s`, s.table, s.arg(1))

	res, err := s.db.ExecContext(ctx, q, now.UTC())
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

func scan(sc scanner) (*sessionx.Session, error) {
	var (
		sess                          sessionx.Session
		status                        string
		device, client, network, data []byte
		renewed                       sql.NullTime
	)

	err := sc.Scan(&sess.ID, &sess.UserID, &status, &device, &client, &network, &data,
		&sess.Locale, &sess.LocaleSource,
		&sess.CreatedAt, &sess.LastSeen, &sess.ExpiresAt, &renewed)
	if err != nil {
		return nil, err
	}

	sess.Status = sessionx.Status(status)
	if renewed.Valid {
		sess.RenewedAt = renewed.Time
	}
	if err := json.Unmarshal(device, &sess.Device); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(client, &sess.Client); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(network, &sess.Network); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &sess.Data); err != nil {
		return nil, err
	}
	return &sess, nil
}
