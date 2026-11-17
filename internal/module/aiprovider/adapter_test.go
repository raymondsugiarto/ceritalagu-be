package aiprovider_test

import (
	"errors"
	"testing"

	"github.com/yourorg/music-api/internal/module/aiprovider"
	"github.com/yourorg/music-api/internal/module/aiprovider/lyria"
	"github.com/yourorg/music-api/internal/module/aiprovider/minimax"
	"github.com/yourorg/music-api/internal/module/aiprovider/muapi"
	"github.com/yourorg/music-api/internal/module/aiprovider/sunoapi"
)

func TestRegistry(t *testing.T) {
	t.Parallel()
	r := aiprovider.NewRegistry()
	minimax.Register(r)
	lyria.Register(r)
	muapi.Register(r)
	sunoapi.Register(r)

	if _, err := r.LyricsStyle(aiprovider.AdapterMiniMaxText); err != nil {
		t.Fatalf("minimax text adapter not registered: %v", err)
	}
	if _, err := r.Music(aiprovider.AdapterMiniMaxMusic); err != nil {
		t.Fatalf("minimax music adapter not registered: %v", err)
	}
	if _, err := r.Music(aiprovider.AdapterLyriaMusic); err != nil {
		t.Fatalf("lyria music adapter not registered: %v", err)
	}
	if _, err := r.Music(aiprovider.AdapterMuapiSuno); err != nil {
		t.Fatalf("muapi suno adapter not registered: %v", err)
	}
	if _, err := r.Music(aiprovider.AdapterSunoAPISuno); err != nil {
		t.Fatalf("sunoapi suno adapter not registered: %v", err)
	}
	if _, err := r.Music("nope"); !errors.Is(err, aiprovider.ErrUnknownAdapter) {
		t.Fatalf("want ErrUnknownAdapter, got %v", err)
	}
}
