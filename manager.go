package sessionx

import (
	"context"
	"errors"
	"sort"
)

// Manager collects sessions from request facts and enforces their
// lifecycle. It is safe for concurrent use as long as the Store is.
type Manager struct {
	store Store
	opts  Options
}

// NewManager returns a Manager over the given store.
func NewManager(store Store, opts Options) *Manager {
	return &Manager{store: store, opts: opts}
}

// Input is what a transport hands over about one request. Every framework
// adapter fills this same struct, which is why the adapters stay thin.
type Input struct {
	// UserID is the subject the session belongs to. Empty is allowed and
	// means an anonymous session.
	UserID string

	// UserAgent is the raw User-Agent header.
	UserAgent string

	// RemoteAddr is the socket peer, with or without a port.
	RemoteAddr string

	// Forwarded is the X-Forwarded-For header, acted on only when the peer
	// is a trusted proxy.
	Forwarded string

	// Device and Client are the already-collected identification, filled by
	// InputFrom when a full *http.Request was available so that Client
	// Hints are not thrown away. Left zero, Collect derives both from
	// UserAgent alone.
	Device Device
	Client Client

	// Locale and LocaleSource are the resolved language and its provenance,
	// filled by InputFrom when Options.Locale is set.
	Locale       string
	LocaleSource string

	// Data is the caller's payload, stored untouched.
	Data map[string]any
}

// Collect opens a session from the facts of a request: it issues an id,
// resolves the device and the network, sets the deadlines, enforces the
// per-user cap and saves the result.
func (m *Manager) Collect(ctx context.Context, in Input) (*Session, error) {
	id, err := NewID()
	if err != nil {
		return nil, err
	}

	device, client := in.Device, in.Client
	if !device.Known() && device.Raw == "" {
		device, client = DeviceFromUA(in.UserAgent)
	}

	now := m.opts.now()
	s := &Session{
		ID:           id,
		UserID:       in.UserID,
		Status:       StatusActive,
		Device:       device,
		Client:       client,
		Network:      NetworkFrom(in.RemoteAddr, in.Forwarded, m.opts.TrustedProxies),
		Locale:       in.Locale,
		LocaleSource: in.LocaleSource,
		Data:         in.Data,
		CreatedAt:    now,
		LastSeen:     now,
		ExpiresAt:    now.Add(m.opts.ttl()),
	}

	if err := m.enforceCap(ctx, in.UserID); err != nil {
		return nil, err
	}
	if err := m.store.Save(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

// Get loads a session and refuses it if the lifecycle says it is over.
//
// An expired session is recorded as expired in the store rather than
// deleted: "this session ended because it timed out" is information an
// audit log wants, and a deleted row cannot carry it.
func (m *Manager) Get(ctx context.Context, id string) (*Session, error) {
	s, err := m.store.Load(ctx, id)
	if err != nil {
		return nil, err
	}

	switch s.Status {
	case StatusRevoked:
		return nil, ErrRevoked
	case StatusExpired:
		return nil, ErrExpired
	}

	now := m.opts.now()
	if s.Expired(now) {
		if err := s.MoveTo(StatusExpired); err == nil {
			_ = m.store.Save(ctx, s)
		}
		return nil, ErrExpired
	}

	if s.IdleSince(now, m.opts.IdleTimeout) && s.Status == StatusActive {
		if err := s.MoveTo(StatusIdle); err == nil {
			_ = m.store.Save(ctx, s)
		}
	}
	return s, nil
}

// Touch marks a session as seen now, reviving it from idle. It is what a
// middleware calls on every request.
func (m *Manager) Touch(ctx context.Context, id string) (*Session, error) {
	s, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	if s.Status == StatusIdle {
		if err := s.MoveTo(StatusActive); err != nil {
			return nil, err
		}
	}
	s.LastSeen = m.opts.now()

	if err := m.store.Save(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

// Renew rotates the session id and restarts the absolute lifetime, carrying
// the payload, the device and the network across.
//
// Rotating on privilege change — sign-in above all — is the defence against
// session fixation: an id an attacker planted before the login is not the id
// that carries the session after it.
//
// The old session is revoked and that revocation is persisted before the
// replacement is ever saved. That ordering is deliberate, not incidental:
// if the revoke-save fails, nothing has changed yet and the caller is told,
// which is fine because no rotation was claimed. If the replacement's save
// then fails, the old id is already revoked, so the caller ends up signed
// out rather than left holding a still-live pre-rotation id.
//
// The old row is kept, revoked, rather than deleted. A revoked row is what
// stops a request that loaded the old id before the rotation from saving it
// back to life afterwards: the store refuses to overwrite a terminal
// session, but it cannot refuse to recreate a deleted one. The row goes when
// the store's own expiry or GC removes it.
func (m *Manager) Renew(ctx context.Context, id string) (*Session, error) {
	old, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	newID, err := NewID()
	if err != nil {
		return nil, err
	}

	now := m.opts.now()
	next := old.Clone()
	next.ID = newID
	next.Status = StatusActive
	next.LastSeen = now
	next.RenewedAt = now
	next.ExpiresAt = now.Add(m.opts.ttl())

	if err := old.MoveTo(StatusRevoked); err != nil {
		return nil, err
	}
	// The store refuses to overwrite a terminal session, so of two Renews
	// racing on one id only the first gets past this line; the second sees
	// ErrRevoked and no second replacement is minted.
	if err := m.store.Save(ctx, old); err != nil {
		return nil, err
	}
	if err := m.store.Save(ctx, next); err != nil {
		return nil, err
	}
	return next, nil
}

// Revoke ends a session deliberately. Revoking an already-terminal session
// is a no-op, not an error: a double sign-out is not a failure.
func (m *Manager) Revoke(ctx context.Context, id string) error {
	s, err := m.store.Load(ctx, id)
	if err != nil {
		return err
	}
	if s.Status.Terminal() {
		return nil
	}
	if err := s.MoveTo(StatusRevoked); err != nil {
		return err
	}
	if err := m.store.Save(ctx, s); err != nil && !endedErr(err) {
		return err
	}
	return nil
}

// endedErr reports whether err says the session was already over — a
// racing sign-out or expiry got there first, which for a revoke is success.
func endedErr(err error) bool {
	return errors.Is(err, ErrRevoked) || errors.Is(err, ErrExpired)
}

// RevokeUser ends every live session a user holds — "sign out everywhere" —
// and returns how many were ended.
func (m *Manager) RevokeUser(ctx context.Context, userID string) (int, error) {
	sessions, err := m.store.ListByUser(ctx, userID)
	if err != nil {
		return 0, err
	}

	n := 0
	for _, s := range sessions {
		if s.Status.Terminal() {
			continue
		}
		if err := s.MoveTo(StatusRevoked); err != nil {
			continue
		}
		if err := m.store.Save(ctx, s); err != nil {
			if endedErr(err) {
				continue
			}
			return n, err
		}
		n++
	}
	return n, nil
}

// ListUser returns the user's live sessions, most recently seen first. It is
// what an "active devices" screen renders.
func (m *Manager) ListUser(ctx context.Context, userID string) ([]*Session, error) {
	all, err := m.store.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := m.opts.now()
	live := make([]*Session, 0, len(all))
	for _, s := range all {
		if s.Status.Terminal() || s.Expired(now) {
			continue
		}
		live = append(live, s)
	}
	sort.Slice(live, func(i, j int) bool { return live[i].LastSeen.After(live[j].LastSeen) })
	return live, nil
}

// enforceCap revokes the least recently seen sessions until the user is one
// below MaxPerUser, so the session about to be collected fits.
func (m *Manager) enforceCap(ctx context.Context, userID string) error {
	if m.opts.MaxPerUser <= 0 || userID == "" {
		return nil
	}

	live, err := m.ListUser(ctx, userID)
	if err != nil {
		// A stateless store cannot enumerate, so it cannot cap either.
		// That is a documented limit of that backend, not a failure here.
		if errors.Is(err, ErrUnsupported) {
			return nil
		}
		return err
	}

	for i := m.opts.MaxPerUser - 1; i < len(live); i++ {
		if err := m.Revoke(ctx, live[i].ID); err != nil {
			return err
		}
	}
	return nil
}
