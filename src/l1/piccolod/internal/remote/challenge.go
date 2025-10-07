package remote

import (
    "net/http"
    "strings"
    "sync"
)

// ChallengeManager stores HTTP-01 token payloads and serves them over HTTP.
type ChallengeManager struct {
    mu     sync.RWMutex
    tokens map[string]string
}

func NewChallengeManager() *ChallengeManager {
    return &ChallengeManager{tokens: make(map[string]string)}
}

// Put registers a token->keyAuthorization mapping.
func (m *ChallengeManager) Put(token, value string) {
    if token == "" { return }
    m.mu.Lock()
    m.tokens[token] = value
    m.mu.Unlock()
}

// Delete removes a token mapping.
func (m *ChallengeManager) Delete(token string) {
    if token == "" { return }
    m.mu.Lock()
    delete(m.tokens, token)
    m.mu.Unlock()
}

// Handler returns an http.Handler that serves /.well-known/acme-challenge/*.
func (m *ChallengeManager) Handler() http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Expect path like "/.well-known/acme-challenge/<token>"
        p := r.URL.Path
        idx := strings.LastIndex(p, "/")
        if idx == -1 || idx+1 >= len(p) {
            http.NotFound(w, r)
            return
        }
        token := p[idx+1:]
        m.mu.RLock()
        val, ok := m.tokens[token]
        m.mu.RUnlock()
        if !ok {
            http.NotFound(w, r)
            return
        }
        w.Header().Set("Content-Type", "text/plain; charset=utf-8")
        _, _ = w.Write([]byte(val))
    })
}

