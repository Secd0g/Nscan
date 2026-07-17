package tokenstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"

	"github.com/yourname/nscan/internal/server/repositories"
)

const settingsKey = "node_auth_token"

type Store struct {
	repo     *repositories.SettingsRepo
	fallback string
	mu       sync.RWMutex
	cached   string
}

func New(repo *repositories.SettingsRepo, fallbackToken string) *Store {
	return &Store{repo: repo, fallback: fallbackToken}
}

func (s *Store) Init(ctx context.Context) {
	val, err := s.repo.GetValue(ctx, settingsKey)
	if err != nil || val == "" {
		token := s.fallback
		if token == "" {
			token = generateToken()
		}
		s.mu.Lock()
		s.cached = token
		s.mu.Unlock()
		_ = s.repo.SetValue(ctx, settingsKey, token)
		return
	}
	s.mu.Lock()
	s.cached = val
	s.mu.Unlock()
}

func (s *Store) Get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cached
}

func (s *Store) Regenerate(ctx context.Context) (string, error) {
	token := generateToken()
	if err := s.repo.SetValue(ctx, settingsKey, token); err != nil {
		return "", err
	}
	s.mu.Lock()
	s.cached = token
	s.mu.Unlock()
	return token, nil
}

func generateToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
