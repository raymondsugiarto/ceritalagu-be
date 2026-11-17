package song_test

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/guest"
	"github.com/yourorg/music-api/internal/module/questionnaire"
	"github.com/yourorg/music-api/internal/module/song"
)

// memRepo is an in-memory song.Repository for service and handler tests.
type memRepo struct {
	mu    sync.Mutex
	jobs  map[string]song.Job
	songs map[string]song.Song
	ipOf  map[string]string // guest session -> ip hash
}

func newMemRepo() *memRepo {
	return &memRepo{jobs: map[string]song.Job{}, songs: map[string]song.Song{}, ipOf: map[string]string{}}
}

func (r *memRepo) CreateJob(_ context.Context, j song.Job) (song.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j.ID, j.CreatedAt = uuid.NewString(), time.Now().UTC()
	r.jobs[j.ID] = j
	return j, nil
}

func (r *memRepo) GetJob(_ context.Context, id string) (song.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return song.Job{}, song.ErrJobNotFound
	}
	return j, nil
}

func (r *memRepo) UpdateJob(_ context.Context, j song.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs[j.ID] = j
	return nil
}

func (r *memRepo) UnlockSong(_ context.Context, id, owner string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.songs[id]
	if !ok {
		return song.ErrSongNotFound
	}
	if !s.IsUnlocked {
		s.UnlockedAt = &at
	}
	s.IsUnlocked, s.ExpiresAt = true, nil
	if s.OwnerUserID == "" {
		s.OwnerUserID = owner
	}
	r.songs[id] = s
	return nil
}

func (r *memRepo) GetSong(_ context.Context, id string) (song.Song, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.songs[id]
	if !ok {
		return song.Song{}, song.ErrSongNotFound
	}
	return s, nil
}

func (r *memRepo) SongByJob(_ context.Context, jobID string) (song.Song, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.songs {
		if s.JobID == jobID {
			return s, nil
		}
	}
	return song.Song{}, song.ErrSongNotFound
}

func (r *memRepo) CountGuestJobsSince(_ context.Context, guestID, ipHash string, since time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, j := range r.jobs {
		sameVisitor := j.GuestSessionID == guestID || (ipHash != "" && r.ipOf[j.GuestSessionID] == ipHash)
		if sameVisitor && j.RegenerateOfJobID == "" && !j.CreatedAt.Before(since) {
			n++
		}
	}
	return n, nil
}

func (r *memRepo) CountRegenerates(_ context.Context, root string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, j := range r.jobs {
		if j.RegenerateOfJobID == root {
			n++
		}
	}
	return n, nil
}

// finish marks a job completed and attaches a song.
func (r *memRepo) finish(jobID string, unlocked bool) song.Song {
	r.mu.Lock()
	defer r.mu.Unlock()
	j := r.jobs[jobID]
	j.Status = song.JobCompleted
	r.jobs[jobID] = j
	s := song.Song{
		ID: uuid.NewString(), JobID: jobID, GuestSessionID: j.GuestSessionID, Title: "Untuk Nadia",
		Lyrics: "[Verse]\nsatu\n[Chorus]\nNadia\n[Verse]\ndua\n[Bridge]\ntiga", PreviewAudioKey: "songs/x/preview.mp3",
		IsUnlocked: unlocked,
	}
	r.songs[s.ID] = s
	return s
}

type ownedAnswers map[string]string // questionnaire id -> guest session id

func (o ownedAnswers) Owned(_ context.Context, v guest.Visitor, id string) (questionnaire.Questionnaire, error) {
	if o[id] != v.SessionID {
		return questionnaire.Questionnaire{}, apperror.NotFound("questionnaire not found")
	}
	return questionnaire.Questionnaire{ID: id, GuestSessionID: v.SessionID}, nil
}

type fakeQueue struct {
	mu   sync.Mutex
	ids  []string
	fail bool
}

func (q *fakeQueue) Enqueue(_ context.Context, id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.fail {
		return errors.New("redis down")
	}
	q.ids = append(q.ids, id)
	return nil
}

type fakeAudio struct{}

func (fakeAudio) Get(_ context.Context, key string) ([]byte, error) { return []byte("MP3:" + key), nil }
