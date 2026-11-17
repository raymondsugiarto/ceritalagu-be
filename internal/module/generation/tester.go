package generation

import (
	"context"
	"fmt"

	"github.com/yourorg/music-api/internal/module/aiprovider"
	"github.com/yourorg/music-api/internal/module/prompt"
	"github.com/yourorg/music-api/internal/module/questionnaire"
)

// LyricsTester runs one lyrics & style (or music) call for the admin prompt playground, on the same active
// provider and with the same voice fix-up as the pipeline, but without retries or a job.
type LyricsTester struct {
	provs    Providers
	registry *aiprovider.Registry
}

// NewLyricsTester builds the tester.
func NewLyricsTester(provs Providers, registry *aiprovider.Registry) *LyricsTester {
	return &LyricsTester{provs: provs, registry: registry}
}

// TestLyrics implements prompt.LyricsTester.
func (t *LyricsTester) TestLyrics(ctx context.Context, req aiprovider.LyricsStyleRequest, voice questionnaire.Voice) (aiprovider.LyricsStyleResult, prompt.TesterInfo, error) {
	prov, err := t.provs.Active(ctx, aiprovider.CategoryLyricsStyle)
	if err != nil {
		return aiprovider.LyricsStyleResult{}, prompt.TesterInfo{}, fmt.Errorf("%w: %w", prompt.ErrNoProvider, err)
	}
	info := prompt.TesterInfo{Provider: prov.Name, Model: prov.Config.Model}
	adapter, err := t.registry.LyricsStyle(prov.AdapterType)
	if err != nil {
		return aiprovider.LyricsStyleResult{}, info, fmt.Errorf("%w: %w", prompt.ErrNoProvider, err)
	}
	res, err := adapter.GenerateLyricsStyle(ctx, req, prov.Config)
	if err != nil {
		return res, info, err
	}
	return applyVoice(res, voice), info, nil
}

// TestMusic implements prompt.LyricsTester: one music call on the active provider, no retries, no storage.
func (t *LyricsTester) TestMusic(ctx context.Context, req aiprovider.MusicRequest) (aiprovider.MusicResult, prompt.TesterInfo, error) {
	prov, err := t.provs.Active(ctx, aiprovider.CategoryMusic)
	if err != nil {
		return aiprovider.MusicResult{}, prompt.TesterInfo{}, fmt.Errorf("%w: %w", prompt.ErrNoMusicProvider, err)
	}
	info := prompt.TesterInfo{Provider: prov.Name, Model: prov.Config.Model}
	adapter, err := t.registry.Music(prov.AdapterType)
	if err != nil {
		return aiprovider.MusicResult{}, info, fmt.Errorf("%w: %w", prompt.ErrNoMusicProvider, err)
	}
	res, err := adapter.GenerateMusic(ctx, req, prov.Config)
	return res, info, err
}

var _ prompt.LyricsTester = (*LyricsTester)(nil)
