package acme

import (
	"sync"
	"time"
)

// http01TokenTTL bounds how long a challenge token stays servable if an order is
// abandoned mid-flight (crash, cancelled context). Orders normally delete their
// tokens as soon as validation finishes; this is the backstop so a stale token
// cannot be served forever.
const http01TokenTTL = time.Hour

// HTTP01Store holds the key authorizations for in-flight HTTP-01 orders. The
// ACME manager owns it and the data plane reads it: a plaintext request for
// /.well-known/acme-challenge/<token> is answered from here before any host
// routing, force-SSL redirect, or auth runs.
//
// Nothing is persisted - tokens only have to outlive the order that created them.
type HTTP01Store struct {
	mu     sync.Mutex
	tokens map[string]http01Entry
	now    func() time.Time // overridable in tests
}

type http01Entry struct {
	keyAuth string
	expires time.Time
}

// NewHTTP01Store returns an empty store.
func NewHTTP01Store() *HTTP01Store {
	return &HTTP01Store{tokens: map[string]http01Entry{}, now: time.Now}
}

// Put registers a token's key authorization for ttl (0 selects the default).
func (s *HTTP01Store) Put(token, keyAuth string, ttl time.Duration) {
	if token == "" {
		return
	}
	if ttl <= 0 {
		ttl = http01TokenTTL
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = http01Entry{keyAuth: keyAuth, expires: s.now().Add(ttl)}
}

// Delete drops the given tokens (safe for tokens that were never present).
func (s *HTTP01Store) Delete(tokens ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range tokens {
		delete(s.tokens, t)
	}
}

// KeyAuth returns the key authorization for token, or false when the token is
// unknown or expired. Expired entries are dropped on the way out so an abandoned
// order cannot leak an entry for the process's lifetime.
func (s *HTTP01Store) KeyAuth(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.tokens[token]
	if !ok {
		return "", false
	}
	if !s.now().Before(e.expires) {
		delete(s.tokens, token)
		return "", false
	}
	return e.keyAuth, true
}

// Len reports how many live tokens are registered (test/diagnostic helper).
func (s *HTTP01Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tokens)
}
