package thread

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/topfreegames/pitaya/v2/logger"
	syncx "github.com/topfreegames/pitaya/v2/sync"
)

const (
	OPENED = iota
	CLOSED
)

const (
	DefaultAntsPoolSize      = math.MaxInt32
	DefaultCleanIntervalTime = time.Second
)

var (
	ErrInvalidPoolExpiry = errors.New("invalid expiry for pool")
	ErrPoolClosed        = errors.New("this pool has been closed")
	ErrTimeout           = errors.New("operation timed out")
	ErrTaskIdEmpty       = errors.New("task id empty")
)

type Pool struct {
	capacity int32
	running  int32
	lock     sync.Locker
	state    int32
	cond     *sync.Cond

	taskWorders map[string]worker
	workers     *workerStack
	workerCache sync.Pool

	purgeDone int32
	purgeCtx  context.Context
	stopPurge context.CancelFunc

	ticktockDone int32
	ticktockCtx  context.Context
	stopTicktock context.CancelFunc
	now          atomic.Value
	waiting      int32

	allDone chan struct{}
	once    *sync.Once

	workerChanCap  int32
	expiryDuration time.Duration

	lastDumpTime    int64         // 记录上次 dump 时间（Unix秒）
	minDumpInterval time.Duration // 最小打印间隔
}

func NewPool(size int, workerChanCap int32, expiryDuration time.Duration) (*Pool, error) {
	if size <= 0 {
		size = -1
	}
	if workerChanCap <= 0 {
		if runtime.GOMAXPROCS(0) == 1 {
			workerChanCap = 0
		} else {
			workerChanCap = 1
		}
	}
	if expiryDuration < 0 {
		return nil, ErrInvalidPoolExpiry
	} else if expiryDuration == 0 {
		expiryDuration = DefaultCleanIntervalTime
	}
	p := &Pool{
		capacity:        int32(size),
		allDone:         make(chan struct{}),
		lock:            syncx.NewSpinLock(),
		once:            &sync.Once{},
		workers:         newWorkerStack(0),
		taskWorders:     make(map[string]worker),
		workerChanCap:   workerChanCap,
		expiryDuration:  expiryDuration,
		minDumpInterval: time.Second,
	}
	p.workerCache.New = func() any {
		return &goWorker{
			pool: p,
			task: make(chan func(string), workerChanCap),
		}
	}
	p.cond = sync.NewCond(p.lock)
	p.goPurge()
	p.goTicktock()

	return p, nil
}

func (p *Pool) SubmitWithTimeout(id string, timeout time.Duration, task func(string)) error {
	result := make(chan error, 1)

	go func() {
		err := p.Submit(id, task)
		result <- err
	}()

	select {
	case err := <-result:
		return err
	case <-time.After(timeout):
		p.dumpGoroutines(id)
		return fmt.Errorf("submit timeout after %v", timeout)
	}
}

func (p *Pool) Submit(id string, task func(string)) error {
	if p.IsClosed() {
		return ErrPoolClosed
	}
	if id == "" {
		return ErrTaskIdEmpty
	}
	worker, err := p.retrieveWorker(id)
	if err != nil {
		return err
	}
	worker.inputFunc(task)
	return nil
}

func (p *Pool) Waiting() int {
	return int(atomic.LoadInt32(&p.waiting))
}

func (p *Pool) IsClosed() bool {
	return atomic.LoadInt32(&p.state) == CLOSED
}

func (p *Pool) Cap() int {
	return int(atomic.LoadInt32(&p.capacity))
}

func (p *Pool) Running() int {
	return int(atomic.LoadInt32(&p.running))
}

func (p *Pool) Release() {
	if !atomic.CompareAndSwapInt32(&p.state, OPENED, CLOSED) {
		return
	}

	if p.stopPurge != nil {
		p.stopPurge()
		p.stopPurge = nil
	}
	if p.stopTicktock != nil {
		p.stopTicktock()
		p.stopTicktock = nil
	}

	p.lock.Lock()
	p.workers.reset()
	clear(p.taskWorders)
	p.lock.Unlock()

	p.cond.Broadcast()
}

func (p *Pool) ReleaseTimeout(timeout time.Duration) error {
	if p.IsClosed() || p.stopPurge == nil || p.stopTicktock == nil {
		return ErrPoolClosed
	}

	p.Release()

	if p.Running() == 0 {
		p.once.Do(func() {
			close(p.allDone)
		})
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			return ErrTimeout
		case <-p.allDone:
			<-p.purgeCtx.Done()
			<-p.ticktockCtx.Done()
			if p.Running() == 0 &&
				(atomic.LoadInt32(&p.purgeDone) == 1) &&
				atomic.LoadInt32(&p.ticktockDone) == 1 {
				return nil
			}
		}
	}
}

func (p *Pool) Reboot() {
	if atomic.CompareAndSwapInt32(&p.state, CLOSED, OPENED) {
		atomic.StoreInt32(&p.purgeDone, 0)
		p.goPurge()
		atomic.StoreInt32(&p.ticktockDone, 0)
		p.goTicktock()
		p.allDone = make(chan struct{})
		p.once = &sync.Once{}
	}
}

func (p *Pool) addWaiting(delta int) {
	atomic.AddInt32(&p.waiting, int32(delta))
}

func (p *Pool) addRunning(delta int) int {
	return int(atomic.AddInt32(&p.running, int32(delta)))
}

func (p *Pool) goPurge() {
	p.purgeCtx, p.stopPurge = context.WithCancel(context.Background())
	go p.purgeStaleWorkers()
}

func (p *Pool) purgeStaleWorkers() {
	ticker := time.NewTicker(p.expiryDuration)

	defer func() {
		ticker.Stop()
		atomic.StoreInt32(&p.purgeDone, 1)
	}()

	purgeCtx := p.purgeCtx
	for {
		select {
		case <-purgeCtx.Done():
			return
		case <-ticker.C:
		}

		if p.IsClosed() {
			break
		}

		var isDormant bool
		p.lock.Lock()
		staleWorkers := p.workers.refresh(p.expiryDuration)
		n := p.Running()
		isDormant = n == 0 || n == len(staleWorkers)
		p.lock.Unlock()

		for i := range staleWorkers {
			staleWorkers[i].finish()
			staleWorkers[i] = nil
		}

		if isDormant && p.Waiting() > 0 {
			p.cond.Broadcast()
		}
	}
}

func (p *Pool) goTicktock() {
	p.now.Store(time.Now())
	p.ticktockCtx, p.stopTicktock = context.WithCancel(context.Background())
	go p.ticktock()
}

const nowTimeUpdateInterval = 500 * time.Millisecond

func (p *Pool) ticktock() {
	ticker := time.NewTicker(nowTimeUpdateInterval)
	defer func() {
		ticker.Stop()
		atomic.StoreInt32(&p.ticktockDone, 1)
	}()

	ticktockCtx := p.ticktockCtx
	for {
		select {
		case <-ticktockCtx.Done():
			return
		case <-ticker.C:
		}

		if p.IsClosed() {
			break
		}

		p.now.Store(time.Now())
	}
}

func (p *Pool) nowTime() time.Time {
	return p.now.Load().(time.Time)
}

// 获取一个worker
func (p *Pool) retrieveWorker(id string) (w worker, err error) {
	p.lock.Lock()

retry:
	// 任务协程
	if w = p.taskWorders[id]; w != nil {
		w.addRef(1)
		p.lock.Unlock()
		return
	}
	// 活跃协程
	if w = p.workers.detach(); w != nil {
		w.setId(id)
		p.taskWorders[id] = w
		w.addRef(1)
		p.lock.Unlock()
		return
	}
	// 对象池
	if capacity := p.Cap(); capacity == -1 || capacity > p.Running() {
		w = p.workerCache.Get().(worker)
		w.run()
		w.setId(id)
		p.taskWorders[id] = w
		w.addRef(1)
		p.lock.Unlock()
		return
	}
	// 挂起等待
	p.addWaiting(1)
	p.cond.Wait()
	p.addWaiting(-1)

	if p.IsClosed() {
		p.lock.Unlock()
		return nil, ErrPoolClosed
	}

	goto retry
}

func (p *Pool) revertWorker(worker worker) bool {
	if worker.addRef(-1) > 0 {
		return true
	}
	if p.IsClosed() {
		p.cond.Broadcast()
		return false
	}

	p.lock.Lock()
	if worker.getRef() > 0 {
		p.lock.Unlock()
		return true
	}
	worker.setLastUsedTime(p.nowTime())

	delete(p.taskWorders, worker.getId())
	worker.setId("")
	if capacity := p.Cap(); (capacity > 0 && p.Running() > capacity) || p.IsClosed() {
		p.lock.Unlock()
		p.cond.Broadcast()
		return false
	}

	if err := p.workers.insert(worker); err != nil {
		p.lock.Unlock()
		return false
	}
	p.cond.Signal()
	p.lock.Unlock()

	return true
}

func (p *Pool) dumpGoroutines(id string) {
	now := time.Now().Unix()
	last := atomic.LoadInt64(&p.lastDumpTime)
	if now-last < int64(p.minDumpInterval.Seconds()) {
		return
	}
	if !atomic.CompareAndSwapInt64(&p.lastDumpTime, last, now) {
		return
	}
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	logger.Log.Errorf("=== Goroutine Dump Start (taskId=%s) ===\n%s\n=== Goroutine Dump End ===", id, buf[:n])
}
