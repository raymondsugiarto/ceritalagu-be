package generation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskGenerateSong is the Asynq task type that runs Pipeline.Run for one song_job.
const TaskGenerateSong = "song:generate"

type generatePayload struct {
	JobID string `json:"job_id"`
}

// Enqueuer puts song jobs on the Asynq queue. The task ID is the job ID, so enqueueing the
// same job twice is a no-op.
type Enqueuer struct {
	client   *asynq.Client
	queue    string
	timeout  time.Duration
	maxRetry int
}

// NewEnqueuer builds an enqueuer on an existing Asynq client.
func NewEnqueuer(c *asynq.Client, queue string, timeout time.Duration, maxRetry int) *Enqueuer {
	return &Enqueuer{client: c, queue: queue, timeout: timeout, maxRetry: maxRetry}
}

// Enqueue schedules jobID for generation.
func (e *Enqueuer) Enqueue(ctx context.Context, jobID string) error {
	payload, err := json.Marshal(generatePayload{JobID: jobID})
	if err != nil {
		return fmt.Errorf("encode task payload: %w", err)
	}
	task := asynq.NewTask(TaskGenerateSong, payload,
		asynq.TaskID(jobID), asynq.Queue(e.queue), asynq.Timeout(e.timeout), asynq.MaxRetry(e.maxRetry))
	if _, err := e.client.EnqueueContext(ctx, task); err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
		return fmt.Errorf("enqueue song job %s: %w", jobID, err)
	}
	return nil
}

// Runner runs one job (Pipeline).
type Runner interface {
	Run(ctx context.Context, jobID string) error
}

// NewTaskHandler adapts a Runner to Asynq. Bad payloads and jobs already saved as failed are not
// retried; anything else (DB down, worker crash) is redelivered by Asynq and resumes safely.
func NewTaskHandler(r Runner) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		var p generatePayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil || p.JobID == "" {
			return fmt.Errorf("invalid %s payload %q: %w", TaskGenerateSong, t.Payload(), asynq.SkipRetry)
		}
		err := r.Run(ctx, p.JobID)
		var failed *FailedError
		if errors.As(err, &failed) {
			return fmt.Errorf("%w: %w", err, asynq.SkipRetry)
		}
		return err
	}
}
