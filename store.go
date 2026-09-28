package sessionx

import "context"

// Store is where sessions live between requests.
//
// Implementations must be safe for concurrent use. Load returns ErrNotFound
// and nothing else when the id is unknown — an empty session with a nil
// error is never a valid answer.
//
// ListByUser and DeleteByUser are in the interface rather than in an
// optional extension because an application that cannot enumerate a user's
// sessions cannot offer "sign out everywhere", and that is table stakes. A
// backend that genuinely cannot honour them returns ErrUnsupported.
type Store interface {
	// Save writes the session, creating or overwriting by ID — unless the
	// stored session is already terminal (revoked or expired). A terminal
	// session is final: Save then leaves it untouched and returns the stored
	// status's Err (ErrRevoked or ErrExpired). The check and the write must
	// be atomic, or a request that loaded the session just before a sign-out
	// can save it back as active and undo the sign-out.
	Save(ctx context.Context, s *Session) error

	// Load reads a session by id, returning ErrNotFound if there is none.
	Load(ctx context.Context, id string) (*Session, error)

	// Delete removes a session. Deleting an absent session is not an error.
	Delete(ctx context.Context, id string) error

	// ListByUser returns every session belonging to userID.
	ListByUser(ctx context.Context, userID string) ([]*Session, error)

	// DeleteByUser removes every session belonging to userID and returns
	// how many were removed.
	DeleteByUser(ctx context.Context, userID string) (int, error)
}

// Tokenizer is implemented by a store that keeps the session in the client
// rather than on the server (cookiestore). Manager.Issue writes the token as
// the cookie value instead of the bare ID, and the middleware reissues it
// after every touch so the cookie carries the current state. Load then
// receives the token as its id.
type Tokenizer interface {
	Token(s *Session) (string, error)
}
