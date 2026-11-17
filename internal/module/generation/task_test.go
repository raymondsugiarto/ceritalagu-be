package generation

import (
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
)

type runnerFunc func(ctx context.Context, jobID string) error

func (f runnerFunc) Run(ctx context.Context, jobID string) error { return f(ctx, jobID) }

func TestTaskHandler(t *testing.T) {
	t.Parallel()
	dbDown := errors.New("db down")
	tests := []struct {
		name      string
		payload   string
		runErr    error
		wantErr   bool
		wantRetry bool
	}{
		{"ok", `{"job_id":"j1"}`, nil, false, false},
		{"bad payload", `nope`, nil, true, false},
		{"empty job id", `{}`, nil, true, false},
		{"job saved as failed", `{"job_id":"j1"}`, &FailedError{JobID: "j1", Err: errors.New("auth")}, true, false},
		{"transient error", `{"job_id":"j1"}`, dbDown, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got string
			h := NewTaskHandler(runnerFunc(func(_ context.Context, id string) error { got = id; return tt.runErr }))
			err := h(context.Background(), asynq.NewTask(TaskGenerateSong, []byte(tt.payload)))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %t", err, tt.wantErr)
			}
			if err != nil && errors.Is(err, asynq.SkipRetry) == tt.wantRetry {
				t.Fatalf("SkipRetry = %t, want retry %t (err %v)", errors.Is(err, asynq.SkipRetry), tt.wantRetry, err)
			}
			if !tt.wantErr && got != "j1" {
				t.Fatalf("runner got job %q", got)
			}
		})
	}
}
