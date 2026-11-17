package generation

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/yourorg/music-api/internal/module/aiprovider"
)

// ProviderStore reads provider rows (aiprovider.GormRepository).
type ProviderStore interface {
	ListActive(ctx context.Context, c aiprovider.Category) ([]aiprovider.AIProvider, error)
}

// Decrypter decrypts stored API keys (secret.Box).
type Decrypter interface {
	Decrypt(encoded string) (string, error)
}

// ProviderResolver implements Providers: picks one active provider row (lowest priority number,
// random among equals) and decrypts its API key.
type ProviderResolver struct {
	store ProviderStore
	box   Decrypter
	intn  func(n int) int
}

// NewProviderResolver builds a resolver.
func NewProviderResolver(store ProviderStore, box Decrypter) *ProviderResolver {
	return &ProviderResolver{store: store, box: box, intn: rand.IntN}
}

// Active returns the provider of c that this call should use. Each call picks again, so providers
// sharing the top priority split jobs roughly evenly.
func (r *ProviderResolver) Active(ctx context.Context, c aiprovider.Category) (ResolvedProvider, error) {
	ps, err := r.store.ListActive(ctx, c)
	if err != nil {
		return ResolvedProvider{}, err
	}
	p, ok := aiprovider.PickByPriority(ps, r.intn)
	if !ok {
		return ResolvedProvider{}, fmt.Errorf("%w: %s", aiprovider.ErrNoActiveProvider, c)
	}
	key, err := r.box.Decrypt(p.APIKeyEncrypted)
	if err != nil {
		return ResolvedProvider{}, fmt.Errorf("decrypt api key of provider %s: %w", p.ID, err)
	}
	return ResolvedProvider{
		ID: p.ID, Name: p.Name, AdapterType: p.AdapterType,
		Config: aiprovider.Config{
			BaseURL: p.BaseURL, Model: p.Model, APIKey: key, Params: p.Params,
			Timeout: time.Duration(p.TimeoutSec) * time.Second, MaxRetries: p.MaxRetries,
		},
	}, nil
}
