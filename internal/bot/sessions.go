package bot

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"yt-music-bot/internal/youtube"
)

const sessionTTL = 30 * time.Minute

// searchSession holds the results of one /music query so pagination and
// download callbacks can reference tracks without re-querying YouTube.
type searchSession struct {
	query     string
	tracks    []youtube.Track
	createdAt time.Time
}

type sessionStore struct {
	mu    sync.Mutex
	items map[string]*searchSession
}

func newSessionStore() *sessionStore {
	s := &sessionStore{items: make(map[string]*searchSession)}
	go s.gc()
	return s
}

func (s *sessionStore) gc() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		s.mu.Lock()
		for k, v := range s.items {
			if now.Sub(v.createdAt) > sessionTTL {
				delete(s.items, k)
			}
		}
		s.mu.Unlock()
	}
}

func (s *sessionStore) put(query string, tracks []youtube.Track) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	sid := hex.EncodeToString(b) // 8 hex chars
	s.mu.Lock()
	s.items[sid] = &searchSession{query: query, tracks: tracks, createdAt: time.Now()}
	s.mu.Unlock()
	return sid
}

func (s *sessionStore) get(sid string) (*searchSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.items[sid]
	return ss, ok
}
