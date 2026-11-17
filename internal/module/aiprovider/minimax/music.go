package minimax

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/yourorg/music-api/internal/module/aiprovider"
)

const (
	musicPath = "/v1/music_generation"
	// DefaultMusicModel is used when the admin leaves the model empty.
	DefaultMusicModel = "music-3.0"
	// MaxLyricsChars is the MiniMax lyrics limit for non-instrumental music-3.0 / music-2.6.
	MaxLyricsChars = 3500
	// MaxPromptChars is the MiniMax style prompt limit.
	MaxPromptChars = 2000
	statusDone     = 2
)

type audioSetting struct {
	SampleRate int    `json:"sample_rate"`
	Bitrate    int    `json:"bitrate"`
	Format     string `json:"format"`
}

type musicRequest struct {
	Model        string       `json:"model"`
	Prompt       string       `json:"prompt,omitempty"`
	Lyrics       string       `json:"lyrics,omitempty"`
	OutputFormat string       `json:"output_format"`
	AudioSetting audioSetting `json:"audio_setting"`
}

type musicResponse struct {
	Data struct {
		Audio  string `json:"audio"`
		Status int    `json:"status"`
	} `json:"data"`
	ExtraInfo struct {
		MusicDuration int `json:"music_duration"` // milliseconds (example: 25364 for a ~25 s clip)
	} `json:"extra_info"`
	BaseResp baseResp `json:"base_resp"`
}

// MusicAdapter generates a song through MiniMax music generation (synchronous, hex audio).
// Params (ai_provider.params): sample_rate (16000|24000|32000|44100), bitrate (32000|64000|128000|256000),
// format (mp3|wav). Defaults: 44100 / 256000 / mp3.
type MusicAdapter struct {
	c client
}

// NewMusicAdapter returns the music adapter.
func NewMusicAdapter(h *http.Client) *MusicAdapter { return &MusicAdapter{c: client{http: h}} }

var _ aiprovider.MusicProvider = (*MusicAdapter)(nil)

// GenerateMusic validates MiniMax limits, generates the song and decodes the audio.
func (a *MusicAdapter) GenerateMusic(ctx context.Context, req aiprovider.MusicRequest, cfg aiprovider.Config) (aiprovider.MusicResult, error) {
	if err := validateMusic(req); err != nil {
		return aiprovider.MusicResult{}, err
	}
	setting := audioSetting{
		SampleRate: intParam(cfg.Params, "sample_rate", 44100),
		Bitrate:    intParam(cfg.Params, "bitrate", 256000),
		Format:     stringParam(cfg.Params, "format", "mp3"),
	}
	in := musicRequest{
		Model:        modelOr(cfg.Model, DefaultMusicModel),
		Prompt:       req.StylePrompt,
		Lyrics:       req.Lyrics,
		OutputFormat: "hex",
		AudioSetting: setting,
	}

	var out musicResponse
	if err := a.c.post(ctx, cfg, musicPath, in, &out); err != nil {
		return aiprovider.MusicResult{}, err
	}
	if err := checkBase(out.BaseResp); err != nil {
		return aiprovider.MusicResult{}, err
	}
	if out.Data.Status != statusDone || out.Data.Audio == "" {
		return aiprovider.MusicResult{}, &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName,
			Message: fmt.Sprintf("music not completed (status %d)", out.Data.Status)}
	}
	audio, err := hex.DecodeString(out.Data.Audio)
	if err != nil {
		return aiprovider.MusicResult{}, &aiprovider.Error{Kind: aiprovider.KindInvalidOutput, Provider: providerName, Message: "audio is not valid hex", Err: err}
	}
	return aiprovider.MusicResult{
		Audio:       audio,
		Format:      setting.Format,
		DurationSec: (out.ExtraInfo.MusicDuration + 500) / 1000,
	}, nil
}

// TestConnection checks the API key without generating a song: the request omits the required
// lyrics, so a valid key gets "invalid parameters" (2013) and a bad key gets an auth error.
func (a *MusicAdapter) TestConnection(ctx context.Context, cfg aiprovider.Config) error {
	var out musicResponse
	in := musicRequest{Model: modelOr(cfg.Model, DefaultMusicModel), OutputFormat: "hex",
		AudioSetting: audioSetting{SampleRate: 44100, Bitrate: 256000, Format: "mp3"}}
	if err := a.c.post(ctx, cfg, musicPath, in, &out); err != nil {
		return err
	}
	if out.BaseResp.StatusCode == 0 || out.BaseResp.StatusCode == 2013 {
		return nil
	}
	return checkBase(out.BaseResp)
}

func validateMusic(req aiprovider.MusicRequest) error {
	invalid := func(msg string) error {
		return &aiprovider.Error{Kind: aiprovider.KindInvalidRequest, Provider: providerName, Message: msg}
	}
	switch n := utf8.RuneCountInString(req.Lyrics); {
	case n == 0:
		return invalid("lyrics are required")
	case n > MaxLyricsChars:
		return invalid(fmt.Sprintf("lyrics have %d characters, max %d", n, MaxLyricsChars))
	}
	if n := utf8.RuneCountInString(req.StylePrompt); n > MaxPromptChars {
		return invalid(fmt.Sprintf("style prompt has %d characters, max %d", n, MaxPromptChars))
	}
	return nil
}
