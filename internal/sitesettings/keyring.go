package sitesettings

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/zalando/go-keyring"
)

var ErrMasterKeyNotFound = errors.New("site settings master key not found")

type MasterKeyStore interface {
	Get(context.Context) (string, error)
	Set(context.Context, string) error
}

type OSMasterKeyStore struct {
	service string
	user    string
}

func NewOSMasterKeyStore() *OSMasterKeyStore {
	return &OSMasterKeyStore{service: "lumi.site-settings", user: "master-key-v1"}
}

func (store *OSMasterKeyStore) Get(_ context.Context) (string, error) {
	value, err := keyring.Get(store.service, store.user)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrMasterKeyNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read site settings master key: %w", err)
	}
	return value, nil
}

func (store *OSMasterKeyStore) Set(_ context.Context, value string) error {
	if err := keyring.Set(store.service, store.user, value); err != nil {
		return fmt.Errorf("store site settings master key: %w", err)
	}
	return nil
}

type MemoryMasterKeyStore struct {
	mu       sync.Mutex
	value    string
	GetCount int
	SetCount int
	GetError error
	SetError error
}

func NewMemoryMasterKeyStore() *MemoryMasterKeyStore { return &MemoryMasterKeyStore{} }

func (store *MemoryMasterKeyStore) Get(_ context.Context) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.GetCount++
	if store.GetError != nil {
		return "", store.GetError
	}
	if store.value == "" {
		return "", ErrMasterKeyNotFound
	}
	return store.value, nil
}

func (store *MemoryMasterKeyStore) Set(_ context.Context, value string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.SetCount++
	if store.SetError != nil {
		return store.SetError
	}
	store.value = value
	return nil
}

func (store *MemoryMasterKeyStore) Clear() {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.value = ""
}
