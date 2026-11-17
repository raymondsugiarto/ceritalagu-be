//go:build integration

package generation

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

// Run with a local Redis: REDIS_ADDR=localhost:6379 go test -tags integration ./internal/module/generation
// Uses Redis DB 15 and its own queue so it does not touch app data.
func TestEnqueueAndProcess(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	opt := asynq.RedisClientOpt{Addr: addr, DB: 15}
	const queue = "integration_test"

	got := make(chan string, 1)
	srv := asynq.NewServer(opt, asynq.Config{Concurrency: 1, Queues: map[string]int{queue: 1}})
	mux := asynq.NewServeMux()
	mux.Handle(TaskGenerateSong, NewTaskHandler(runnerFunc(func(_ context.Context, id string) error {
		got <- id
		return nil
	})))
	if err := srv.Start(mux); err != nil {
		t.Skipf("redis not reachable: %v", err)
	}
	defer srv.Shutdown()

	client := asynq.NewClient(opt)
	defer func() { _ = client.Close() }()
	e := NewEnqueuer(client, queue, time.Minute, 0)
	jobID := "it-" + time.Now().Format("150405.000000")
	if err := e.Enqueue(context.Background(), jobID); err != nil {
		t.Fatal(err)
	}
	if err := e.Enqueue(context.Background(), jobID); err != nil {
		t.Fatalf("second enqueue of the same job must be a no-op: %v", err)
	}

	select {
	case id := <-got:
		if id != jobID {
			t.Fatalf("processed %q, want %q", id, jobID)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("task not processed within 10s")
	}
}
