package jobs

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/berzz/searcher/backend/internal/persistence"
	"github.com/berzz/searcher/backend/internal/processing"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct {
	pool        *pgxpool.Pool
	processor   *processing.Service
	concurrency int
	claimCancel context.CancelFunc
	workCancel  context.CancelFunc
	done        chan struct{}
	stopOnce    sync.Once
}

func NewWorker(pool *pgxpool.Pool, processor *processing.Service, concurrency int) *Worker {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Worker{pool: pool, processor: processor, concurrency: concurrency, done: make(chan struct{})}
}

func (w *Worker) Start(parent context.Context) {
	claimCtx, claimCancel := context.WithCancel(parent)
	workCtx, workCancel := context.WithCancel(context.Background())
	w.claimCancel, w.workCancel = claimCancel, workCancel
	var wg sync.WaitGroup
	for i := 0; i < w.concurrency; i++ {
		wg.Add(1)
		go func(workerNumber int) {
			defer wg.Done()
			w.loop(claimCtx, workCtx, workerNumber)
		}(i + 1)
	}
	go func() {
		wg.Wait()
		close(w.done)
	}()
}

func (w *Worker) Stop(ctx context.Context) error {
	w.stopOnce.Do(func() {
		if w.claimCancel != nil {
			w.claimCancel()
		}
	})
	select {
	case <-w.done:
		if w.workCancel != nil {
			w.workCancel()
		}
		return nil
	case <-ctx.Done():
		if w.workCancel != nil {
			w.workCancel()
		}
		return ctx.Err()
	}
}

func (w *Worker) loop(claimCtx, workCtx context.Context, workerNumber int) {
	workerID := time.Now().UTC().Format("20060102T150405.000000000") + "-" + string(rune('a'+workerNumber))
	for {
		job, err := persistence.ClaimEmbeddingJob(claimCtx, w.pool, workerID)
		if errors.Is(err, pgx.ErrNoRows) {
			select {
			case <-claimCtx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}
		if err != nil {
			if claimCtx.Err() != nil {
				return
			}
			slog.Error("processing job claim failed", "worker_id", workerID, "error", err)
			time.Sleep(time.Second)
			continue
		}
		if job.VideoID == nil {
			_ = persistence.FinishJob(context.Background(), w.pool, job.ID, false, errors.New("embed job has no video_id"), nil)
			continue
		}
		started := time.Now()
		slog.Info("processing job started", "job_id", job.ID, "video_id", *job.VideoID, "attempt", job.Attempts, "worker_id", workerID)
		err = w.processor.Process(workCtx, *job.VideoID)
		if err == nil {
			err = persistence.FinishJob(context.Background(), w.pool, job.ID, true, nil, nil)
			slog.Info("processing job completed", "job_id", job.ID, "video_id", *job.VideoID, "duration", time.Since(started))
			continue
		}
		var retryAt *time.Time
		if job.Attempts < 5 {
			next := time.Now().Add(retryDelay(job.Attempts))
			if workCtx.Err() != nil {
				next = time.Now()
			}
			retryAt = &next
		}
		if finishErr := persistence.FinishJob(context.Background(), w.pool, job.ID, false, err, retryAt); finishErr != nil {
			slog.Error("processing job failure could not be recorded", "job_id", job.ID, "video_id", *job.VideoID, "error", finishErr)
		}
		slog.Error("processing job failed", "job_id", job.ID, "video_id", *job.VideoID, "attempt", job.Attempts, "retry", retryAt != nil, "duration", time.Since(started), "error", err)
	}
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<attempt) * time.Second
}
