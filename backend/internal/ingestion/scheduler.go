package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/hibiken/asynq"

	"github.com/mohityadav8/job-find/backend/internal/ingestion/sources"
)

// The scheduler runs ingestion OFF the request path (README §7 non-negotiable).
// Two moving parts:
//
//   - asynq.Scheduler enqueues a "ingest:sync" task on a cron cadence.
//   - asynq.Server (worker) dequeues that task and runs the Runner.
//
// Redis is the broker (config.RedisAddr). Running the API and the worker as
// separate processes (cmd/api vs cmd/ingest) means a heavy sync never competes
// with user traffic for CPU.

const (
	// TaskTypeSync is the Asynq task type for a full ingestion pass.
	TaskTypeSync = "ingest:sync"
	// QueueIngest is the dedicated queue name so ingestion can be scaled/paused
	// independently of any future task types.
	QueueIngest = "ingest"
)

// SyncPayload is the JSON payload carried by a sync task. Empty Queries means
// "use DefaultQueries".
type SyncPayload struct {
	Queries []sources.Query `json:"queries,omitempty"`
}

// NewSyncTask builds an Asynq task for a full sync.
func NewSyncTask(queries []sources.Query) (*asynq.Task, error) {
	payload, err := json.Marshal(SyncPayload{Queries: queries})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskTypeSync, payload, asynq.Queue(QueueIngest)), nil
}

// RegisterScheduler wires a cron entry that enqueues a sync task on the given
// cron spec (e.g. "0 */6 * * *" for every 6 hours). Returns the started
// scheduler; caller is responsible for Shutdown.
func RegisterScheduler(redisAddr, cronSpec string) (*asynq.Scheduler, error) {
	if cronSpec == "" {
		cronSpec = "0 */6 * * *" // every 6 hours by default
	}
	scheduler := asynq.NewScheduler(
		asynq.RedisClientOpt{Addr: redisAddr},
		&asynq.SchedulerOpts{Location: time.UTC},
	)

	task, err := NewSyncTask(nil) // nil → DefaultQueries at run time
	if err != nil {
		return nil, err
	}
	if _, err := scheduler.Register(cronSpec, task); err != nil {
		return nil, fmt.Errorf("registering sync cron: %w", err)
	}
	return scheduler, nil
}

// Worker owns the Asynq server that processes ingestion tasks.
type Worker struct {
	server *asynq.Server
	runner *Runner
}

// NewWorker builds an Asynq worker bound to the ingest queue.
func NewWorker(redisAddr string, runner *Runner, concurrency int) *Worker {
	if concurrency <= 0 {
		concurrency = 4
	}
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency: concurrency,
			Queues:      map[string]int{QueueIngest: 1},
		},
	)
	return &Worker{server: srv, runner: runner}
}

// Run starts processing and blocks until the server is shut down.
func (w *Worker) Run() error {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskTypeSync, w.handleSync)
	return w.server.Run(mux)
}

// Shutdown gracefully stops the worker.
func (w *Worker) Shutdown() { w.server.Shutdown() }

func (w *Worker) handleSync(ctx context.Context, t *asynq.Task) error {
	var p SyncPayload
	if len(t.Payload()) > 0 {
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return fmt.Errorf("sync: bad payload: %w", err)
		}
	}
	queries := p.Queries
	if len(queries) == 0 {
		queries = DefaultQueries()
	}
	log.Printf("worker: starting ingestion sync (%d queries)", len(queries))
	res := w.runner.Run(ctx, queries)
	log.Printf("worker: sync complete — inserted=%d skipped=%d", res.Inserted, res.Skipped)
	return nil
}

// EnqueueOnce enqueues a single immediate sync task (used by the CLI / admin
// trigger to force a run without waiting for the cron cadence).
func EnqueueOnce(redisAddr string, queries []sources.Query) error {
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	defer client.Close()
	task, err := NewSyncTask(queries)
	if err != nil {
		return err
	}
	info, err := client.Enqueue(task)
	if err != nil {
		return err
	}
	log.Printf("enqueued sync task id=%s queue=%s", info.ID, info.Queue)
	return nil
}
