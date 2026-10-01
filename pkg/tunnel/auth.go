package tunnel

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// ClientAuth describes credentials used by a Chameleon client.
// ClientID + ClientSecret select Auth v2. PSK is the legacy v1 fallback.
type ClientAuth struct {
	PSK          string
	ClientID     string
	ClientSecret string
}

func (a ClientAuth) UsesV2() bool {
	return strings.TrimSpace(a.ClientID) != "" || strings.TrimSpace(a.ClientSecret) != ""
}

func (a ClientAuth) Validate() error {
	if a.UsesV2() {
		if err := validateClientID(a.ClientID); err != nil {
			return err
		}
		if err := validateClientSecret(a.ClientSecret); err != nil {
			return err
		}
		return nil
	}
	if strings.TrimSpace(a.PSK) == "" {
		return fmt.Errorf("client credentials are required")
	}
	return nil
}

// ClientCredential is one independently revocable Auth v2 identity.
type ClientCredential struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Secret    string `json:"secret"`
	Enabled   bool   `json:"enabled"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type clientRegistryFile struct {
	Schema  int                `json:"schema"`
	Clients []ClientCredential `json:"clients"`
}

// ClientRegistry stores independently revocable Auth v2 clients.
type ClientRegistry struct {
	mu      sync.RWMutex
	clients map[string]ClientCredential
}

func NewClientRegistry(credentials []ClientCredential) (*ClientRegistry, error) {
	registry := &ClientRegistry{clients: make(map[string]ClientCredential, len(credentials))}
	for _, credential := range credentials {
		credential.ID = strings.TrimSpace(credential.ID)
		credential.Secret = strings.TrimSpace(credential.Secret)
		if err := validateClientID(credential.ID); err != nil {
			return nil, err
		}
		if err := validateClientSecret(credential.Secret); err != nil {
			return nil, fmt.Errorf("client %q: %w", credential.ID, err)
		}
		if credential.ExpiresAt != "" {
			if _, err := time.Parse(time.RFC3339, credential.ExpiresAt); err != nil {
				return nil, fmt.Errorf("client %q expires_at: %w", credential.ID, err)
			}
		}
		if _, exists := registry.clients[credential.ID]; exists {
			return nil, fmt.Errorf("duplicate client id %q", credential.ID)
		}
		registry.clients[credential.ID] = credential
	}
	return registry, nil
}

func LoadClientRegistry(path string) (*ClientRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read clients file: %w", err)
	}
	var file clientRegistryFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse clients file: %w", err)
	}
	if file.Schema != 1 {
		return nil, fmt.Errorf("unsupported clients schema %d", file.Schema)
	}
	return NewClientRegistry(file.Clients)
}

func (r *ClientRegistry) Len() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}

func (r *ClientRegistry) Resolve(id string, now time.Time) (ClientCredential, bool) {
	if r == nil {
		return ClientCredential{}, false
	}
	r.mu.RLock()
	credential, ok := r.clients[strings.TrimSpace(id)]
	r.mu.RUnlock()
	if !ok || !credential.Enabled {
		return ClientCredential{}, false
	}
	if credential.ExpiresAt != "" {
		expires, err := time.Parse(time.RFC3339, credential.ExpiresAt)
		if err != nil || !now.Before(expires) {
			return ClientCredential{}, false
		}
	}
	return credential, true
}

func validateClientID(id string) error {
	id = strings.TrimSpace(id)
	if len(id) < 8 || len(id) > 128 {
		return fmt.Errorf("client id must contain 8-128 characters")
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || strings.ContainsRune("-_.", r) {
			continue
		}
		return fmt.Errorf("client id contains unsupported character %q", r)
	}
	return nil
}

func validateClientSecret(secret string) error {
	secret = strings.TrimSpace(secret)
	if len(secret) < 24 || len(secret) > 256 {
		return fmt.Errorf("client secret must contain 24-256 characters")
	}
	return nil
}
