package thread

import (
	"context"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/topfreegames/pitaya/v2/constants"
	"github.com/topfreegames/pitaya/v2/logger"
)

type TaskEntry struct {
	Ctx  context.Context
	Task func(ctx context.Context)
}

type worker interface {
	getId() string
	setId(string)
	run()
	finish()
	inputFunc(*TaskEntry) error
	getRef() int32
	addRef(int32) int32
	lastUsedTime() time.Time
	setLastUsedTime(time.Time)
}

type goWorker struct {
	id  string
	ref int32

	pool     *Pool
	task     chan *TaskEntry
	lastUsed time.Time
}

func (w *goWorker) run() {
	w.pool.addRunning(1)
	go func() {
		defer func() {
			if w.pool.addRunning(-1) == 0 && w.pool.IsClosed() {
				w.pool.once.Do(func() {
					close(w.pool.allDone)
				})
			}
			// 此处一定是close 不需要清理taskWorkers
			w.id = ""
			w.pool.workerCache.Put(w)
			if p := recover(); p != nil {
				logger.Log.Errorf("worker exits from panic: %v\n%s\n", p, debug.Stack())
			}
			w.pool.cond.Signal()
		}()

		for t := range w.task {
			if t == nil {
				return
			}
			if ok := w.safe(t); !ok {
				return
			}
		}
	}()
}

func (w *goWorker) safe(t *TaskEntry) (ok bool) {
	start := time.Now()
	ctx := context.WithValue(t.Ctx, constants.TaskIDKey, w.id)
	defer func() {
		elapsed := time.Since(start)
		if elapsed > 5*time.Second {
			logger.WithCtx(ctx).Warnf("worker [%s] task execution took too long: %v", w.id, elapsed)
		}
		if p := recover(); p != nil {
			logger.WithCtx(ctx).Errorf("worker exits from panic: %v\n%s\n", p, debug.Stack())
		}
		ok = w.pool.revertWorker(w)
	}()

	t.Task(ctx)
	return
}

func (w *goWorker) setId(id string) {
	w.id = id
}

func (w *goWorker) getId() string {
	return w.id
}

func (w *goWorker) addRef(num int32) int32 {
	return atomic.AddInt32(&w.ref, num)
}

func (w *goWorker) getRef() int32 {
	return atomic.LoadInt32(&w.ref)
}

func (w *goWorker) finish() {
	w.task <- nil
}

func (w *goWorker) lastUsedTime() time.Time {
	return w.lastUsed
}

func (w *goWorker) setLastUsedTime(t time.Time) {
	w.lastUsed = t
}

const timeout = 3 * time.Second

func (w *goWorker) inputFunc(t *TaskEntry) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case w.task <- t:
		return nil
	case <-timer.C:
		logger.WithCtx(t.Ctx).Errorf("inputFunc timeout: task queue for worker [%s] is full or worker goroutine is stuck", w.id)
		if w.pool.checkDump() {
			w.pool.dumpGoroutines(w.id, timeout)
			w.pool.dumpWorkerStatus()
		}
		return ErrTaskRunnerBusy
	}
}
