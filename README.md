# sessionx

Session lifecycle for Go HTTP services: collect a session from a request,
enforce absolute and idle timeouts, rotate on sign-in, revoke one or all of a
user's sessions, and keep them in the store that fits your deployment.

```sh
go get github.com/bakhod1r/sessionx
```

Requires Go 1.26.

## Quick start

```go
store := memory.New() // or redisstore.New, sqlstore.New, cookiestore.New
m := sessionx.NewManager(store, sessionx.Options{
	TTL:         24 * time.Hour,
	IdleTimeout: 30 * time.Minute,
	MaxPerUser:  5,
})

// Sign-in: collect a session for the user and hand the client its cookie.
s, err := m.Collect(r.Context(), m.InputFrom(r, userID))
if err != nil { /* ... */ }
if err := m.Issue(w, s, sessionx.DefaultCookie()); err != nil { /* ... */ }

// Every request: attach the session to the context.
mux := m.Middleware(sessionx.DefaultCookie())(appHandler)

// In a handler.
if s, ok := sessionx.FromContext(r.Context()); ok { /* signed in */ }

// Privilege change: rotate the id (session-fixation defence).
s, err = m.Renew(ctx, s.ID)

// Sign out here, or everywhere.
_ = m.Revoke(ctx, s.ID)
_, _ = m.RevokeUser(ctx, s.UserID)
```

## Lifecycle

`active` → `idle` (past `IdleTimeout`; the next request revives it) →
`expired` (past `TTL`) or `revoked` (deliberately ended). Expired and revoked
are **final**: every store refuses to overwrite them, atomically, so a request
that loaded a session just before a sign-out cannot save it back to life.

`IdleTimeout` marks a session idle; it does not end it. To end idle sessions,
revoke them or use a short `TTL`.

## Stores

| Store | Package | Enumerate / revoke | Notes |
|---|---|---|---|
| Memory | `sessionx/memory` | yes | One process. Call `GC` on your own schedule. |
| Redis | `github.com/bakhod1r/sessionx/redis` | yes | Single node, Sentinel or Cluster. Expiry is Redis's TTL. |
| SQL | `github.com/bakhod1r/sessionx/sqlstore` | yes | PostgreSQL and SQLite (`SchemaPostgres`, `SchemaSQLite`). Call `GC` periodically. Not MySQL. |
| Cookie | `sessionx/cookiestore` | **no** | Stateless: the signed session is the cookie. See below. |

Redis and SQL are separate modules so the core pulls in no database client.

### cookiestore

The whole session is signed (HMAC-SHA256) and stored in the cookie; any
process holding the key serves it, and the middleware reissues the cookie
after each request. There is no server state, so:

- `Revoke`, `RevokeUser`, `ListUser` and `MaxPerUser` return or ignore
  `ErrUnsupported`.
- `Renew` issues a new cookie but cannot invalidate the old one; it stays
  valid until its `ExpiresAt`. Keep `TTL` short.
- The payload is readable by the client. Never put secrets in `Data`.
- Keys must be ≥ 32 random bytes. `cookiestore.New(newKey, oldKey)` rotates:
  the first key signs, all keys verify.

## Cookies

The zero `CookieConfig` is `Secure`, `HttpOnly` and `SameSite=Lax`. For local
HTTP development set `Insecure: true`. On HTTPS-only sites, consider the
`__Host-` name prefix.

## Behind a proxy

Set `Options.TrustedProxies`. `X-Forwarded-For` is honoured only when the
direct peer is a trusted proxy, and it is read right to left, so entries a
client added itself are never used.

## Testing a custom store

`storetest.Run` is the conformance suite every bundled store passes, including
the terminal-is-final rule.

## License

MIT
