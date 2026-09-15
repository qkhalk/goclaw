// Package browse holds the server-side pieces of client-side browsing: the
// sanitized-document relay store and the HTML sanitizer. The gateway fetches
// ONE document per browse (SSRF-checked), sanitizes it, and serves it
// same-origin to the web client's browser panel — every subresource (images,
// CSS, fonts) is loaded by the client directly from the origin site, keeping
// the heavy bandwidth and rendering off the server.
package browse

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ErrDocumentTooLarge signals the fetched document exceeds MaxDocumentBytes.
var ErrDocumentTooLarge = errors.New("browse document exceeds relay size cap")

const (
	// MaxDocumentBytes caps one relayed document. The relay is only meant to
	// carry the HTML itself; larger payloads indicate a mis-targeted URL
	// (binary download etc.) and are refused.
	MaxDocumentBytes = 2 * 1024 * 1024

	// TTL bounds how long a relayed document stays readable. Documents are
	// single-use in practice (one browse → one render); 5 minutes covers slow
	// clients and manual reloads while bounding memory.
	TTL = 5 * time.Minute

	// maxEntries bounds the store (LRU eviction) — ~40MB worst case.
	maxEntries = 20
)

// Entry is one sanitized document awaiting relay to a web client.
type Entry struct {
	HTML        string
	Title       string
	URL         string // originally requested URL
	FinalURL    string // after redirects; the sanitizer's base URL
	ContentType string // sniffed content type for the serve header
	CreatedAt   time.Time
}

// Store is a small in-memory LRU of sanitized documents keyed by opaque id.
// Entries expire by TTL; no persistence — a restart simply drops pending
// browses (the tool's fallback path covers the client).
type Store struct {
	mu      sync.Mutex
	entries map[string]*Entry
	order   []string // insertion order, oldest first
}

// NewStore creates an empty relay store.
func NewStore() *Store {
	return &Store{entries: make(map[string]*Entry)}
}

// Put stores a sanitized document and returns its relay id. Oversized
// documents are rejected so one browse cannot pin memory.
func (s *Store) Put(e *Entry) (string, error) {
	if len(e.HTML) > MaxDocumentBytes {
		return "", ErrDocumentTooLarge
	}
	e.CreatedAt = time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	id := newID()
	s.entries[id] = e
	s.order = append(s.order, id)
	for len(s.order) > maxEntries {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.entries, oldest)
	}
	return id, nil
}

// Get returns a live entry by id. Expired entries are dropped and reported
// as missing.
func (s *Store) Get(id string) (*Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !ok {
		return nil, false
	}
	if time.Since(e.CreatedAt) > TTL {
		delete(s.entries, id)
		return nil, false
	}
	return e, true
}

var fallbackIDCounter atomic.Uint64

func newID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand essentially never fails; degrade to a time+counter id
		// rather than panicking in the browse path.
		return fmt.Sprintf("t%dn%d", time.Now().UnixNano(), fallbackIDCounter.Add(1))
	}
	return hex.EncodeToString(b)
}
