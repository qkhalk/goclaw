package browse

import (
	"strings"
	"testing"
	"time"
)

func TestStorePutGetRoundtrip(t *testing.T) {
	s := NewStore()
	id, err := s.Put(&Entry{HTML: "<html>hi</html>", Title: "Hi", FinalURL: "https://example.com"})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	e, ok := s.Get(id)
	if !ok {
		t.Fatal("Get miss after Put")
	}
	if e.HTML != "<html>hi</html>" || e.Title != "Hi" {
		t.Fatalf("entry mismatch: %+v", e)
	}
}

func TestStoreRejectsOversized(t *testing.T) {
	s := NewStore()
	_, err := s.Put(&Entry{HTML: strings.Repeat("x", MaxDocumentBytes+1)})
	if err != ErrDocumentTooLarge {
		t.Fatalf("err = %v, want ErrDocumentTooLarge", err)
	}
}

func TestStoreExpiresByTTL(t *testing.T) {
	s := NewStore()
	id, _ := s.Put(&Entry{HTML: "x"})
	// Age the entry past the TTL (same-package access).
	s.mu.Lock()
	s.entries[id].CreatedAt = time.Now().Add(-TTL - time.Second)
	s.mu.Unlock()
	if _, ok := s.Get(id); ok {
		t.Fatal("expired entry still served")
	}
}

func TestStoreLRUEviction(t *testing.T) {
	s := NewStore()
	first := ""
	for i := range maxEntries + 3 {
		id, _ := s.Put(&Entry{HTML: "x"})
		if i == 0 {
			first = id
		}
	}
	if _, ok := s.Get(first); ok {
		t.Fatal("oldest entry survived eviction")
	}
	s.mu.Lock()
	count := len(s.entries)
	s.mu.Unlock()
	if count > maxEntries {
		t.Fatalf("entries = %d, want <= %d", count, maxEntries)
	}
}
