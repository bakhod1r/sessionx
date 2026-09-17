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
	// Save writes the session, creating or overwriting by ID.
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
