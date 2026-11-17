package playground

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/pagination"
	"github.com/yourorg/music-api/internal/storage"
)

// Repository is the storage the service needs (GormRepository).
type Repository interface {
	Create(ctx context.Context, s Song) error
	Get(ctx context.Context, id string) (Song, error)
	List(ctx context.Context, p ListParams) ([]Song, int64, error)
	Update(ctx context.Context, s Song) error
	Archive(ctx context.Context, id string, at time.Time) error
}

// Store keeps the audio (storage.Local or storage.S3).
type Store interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Get(ctx context.Context, key string) ([]byte, error)
}

// maxLandingSamples caps the public list.
const maxLandingSamples = 24

// Service holds the playground song rules.
type Service struct {
	repo  Repository
	store Store
	now   func() time.Time
	newID func() string
}

// NewService builds the service.
func NewService(repo Repository, store Store) *Service {
	return &Service{repo: repo, store: store, now: func() time.Time { return time.Now().UTC() }, newID: uuid.NewString}
}

// Save stores the audio at playground/<id>.<format> and records it. Implements prompt.PlaygroundSaver.
func (s *Service) Save(ctx context.Context, adminID string, in SaveInput) (string, error) {
	id := s.newID()
	format := in.Format
	if format == "" {
		format = "mp3"
	}
	key := storage.PlaygroundKey(id, format)
	if err := s.store.Put(ctx, key, in.Audio, ContentType(format)); err != nil {
		return "", fmt.Errorf("store playground audio: %w", err)
	}
	now := s.now()
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = "Playground " + now.Format("2006-01-02 15:04")
	}
	err := s.repo.Create(ctx, Song{
		ID: id, Title: title, Lyrics: in.Lyrics, StylePrompt: in.StylePrompt, AudioKey: key, Format: format,
		DurationSec: in.DurationSec, SizeBytes: len(in.Audio), Provider: in.Provider, Model: in.Model,
		CreatedByAdminID: adminID, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// List is the admin list (all songs, or only the landing ones).
func (s *Service) List(ctx context.Context, in ListSongsInput) (pagination.ListOutput[SongOutput], error) {
	in.Normalize()
	songs, total, err := s.repo.List(ctx, ListParams{LandingOnly: in.Landing, Offset: in.Offset(), Limit: in.PageSize})
	if err != nil {
		return pagination.ListOutput[SongOutput]{}, apperror.Internal(err)
	}
	out := make([]SongOutput, 0, len(songs))
	for _, sg := range songs {
		out = append(out, toSongOutput(sg))
	}
	return pagination.NewList(out, in.Input, total), nil
}

// Update edits the title/caption and the landing-page choice.
func (s *Service) Update(ctx context.Context, id string, in UpdateSongInput) (SongOutput, error) {
	sg, err := s.get(ctx, id)
	if err != nil {
		return SongOutput{}, err
	}
	if in.Title != nil {
		if t := strings.TrimSpace(*in.Title); t != "" {
			sg.Title = t
		}
	}
	if in.Caption != nil {
		sg.Caption = strings.TrimSpace(*in.Caption)
	}
	if in.ShowOnLanding != nil {
		sg.ShowOnLanding = *in.ShowOnLanding
	}
	if in.LandingOrder != nil {
		sg.LandingOrder = *in.LandingOrder
	}
	sg.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, sg); err != nil {
		return SongOutput{}, mapErr(err)
	}
	return toSongOutput(sg), nil
}

// Archive hides a song from the admin list and the landing page (soft delete).
func (s *Service) Archive(ctx context.Context, id string) error {
	return mapErr(s.repo.Archive(ctx, id, s.now()))
}

// Audio returns any playground song's audio (admin).
func (s *Service) Audio(ctx context.Context, id string) ([]byte, string, error) {
	sg, err := s.get(ctx, id)
	if err != nil {
		return nil, "", err
	}
	return s.read(ctx, sg)
}

// Samples is the public landing-page list.
func (s *Service) Samples(ctx context.Context) ([]SampleOutput, error) {
	songs, _, err := s.repo.List(ctx, ListParams{LandingOnly: true, Limit: maxLandingSamples})
	if err != nil {
		return nil, apperror.Internal(err)
	}
	out := make([]SampleOutput, 0, len(songs))
	for _, sg := range songs {
		out = append(out, toSampleOutput(sg))
	}
	return out, nil
}

// SampleAudio returns the audio of a song shown on the landing page; any other id is not found.
func (s *Service) SampleAudio(ctx context.Context, id string) ([]byte, string, error) {
	sg, err := s.get(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if !sg.ShowOnLanding {
		return nil, "", apperror.NotFound("sample not found")
	}
	return s.read(ctx, sg)
}

func (s *Service) get(ctx context.Context, id string) (Song, error) {
	sg, err := s.repo.Get(ctx, id)
	if err != nil {
		return Song{}, mapErr(err)
	}
	return sg, nil
}

func (s *Service) read(ctx context.Context, sg Song) ([]byte, string, error) {
	data, err := s.store.Get(ctx, sg.AudioKey)
	if err != nil {
		return nil, "", apperror.Internal(err)
	}
	return data, ContentType(sg.Format), nil
}

// ContentType maps an audio format to its MIME type.
func ContentType(format string) string {
	switch format {
	case "wav":
		return "audio/wav"
	case "flac":
		return "audio/flac"
	case "m4a", "aac":
		return "audio/mp4"
	case "ogg":
		return "audio/ogg"
	default:
		return "audio/mpeg"
	}
}

func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return apperror.NotFound("playground song not found")
	default:
		return apperror.Internal(err)
	}
}
