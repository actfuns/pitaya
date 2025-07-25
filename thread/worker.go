package thread

import (
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/topfreegames/pitaya/v2/logger"
)

type worker interface {
	getId() string
	setId(string)
	run()
	finish()
	inputFunc(func(string)) error
	getRef() int32
	addRef(int32) int32
	lastUsedTime() time.Time
	setLastUsedTime(time.Time)
}

type goWorker struct {
	id  string
	ref int32

	pool     *Pool
	task     chan func(string)
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

		for fn := range w.task {
			if fn == nil {
				return
			}
			if ok := w.safe(fn); !ok {
				return
			}
		}
	}()
}

func (w *goWorker) safe(fn func(string)) (ok bool) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		if elapsed > 5*time.Second {
			logger.Log.Warnf("worker [%s] task execution took too long: %v", w.id, elapsed)
		}
		if p := recover(); p != nil {
			logger.Log.Errorf("worker exits from panic: %v\n%s\n", p, debug.Stack())
		}
		ok = w.pool.revertWorker(w)
	}()

	fn(w.id)
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

func (w *goWorker) inputFunc(fn func(string)) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case w.task <- fn:
		return nil
	case <-timer.C:
		logger.Log.Errorf("inputFunc timeout: task queue for worker [%s] is full or worker goroutine is stuck", w.id)
		if w.pool.checkDump() {
			w.pool.dumpGoroutines(w.id, timeout)
			w.pool.dumpWorkerStatus()
		}
		return ErrTaskRunnerBusy
	}
}
