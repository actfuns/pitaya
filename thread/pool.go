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

var resultChanPool = sync.Pool{
	New: func() any {
		return make(chan error, 1)
	},
}

type submitRequest struct {
	id     string
	task   func(string)
	result chan error
}

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

	dumplastTime     int64         // 记录上次 dump 时间（Unix秒）
	dumpMiniInterval time.Duration // 最小打印间隔
	chSubmit         chan submitRequest
	submitBufferSize int
	submitDispatch   int
	submitDone       int32
	submitCtx        context.Context
	stopSubmit       context.CancelFunc
}

func NewPool(size int, workerChanCap int32, expiryDuration time.Duration, submitBufferSize int, submitDispatch int) (*Pool, error) {
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
		capacity:         int32(size),
		allDone:          make(chan struct{}),
		lock:             syncx.NewSpinLock(),
		once:             &sync.Once{},
		workers:          newWorkerStack(0),
		taskWorders:      make(map[string]worker),
		workerChanCap:    workerChanCap,
		expiryDuration:   expiryDuration,
		dumpMiniInterval: time.Minute,
		submitBufferSize: submitBufferSize,
		chSubmit:         make(chan submitRequest, submitBufferSize),
		submitDispatch:   submitDispatch,
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
	p.goSubmit()

	return p, nil
}

func (p *Pool) SubmitWithTimeout(id string, timeout time.Duration, task func(string)) error {
	result := resultChanPool.Get().(chan error)
	req := submitRequest{
		id:     id,
		task:   task,
		result: result,
	}
	select {
	case p.chSubmit <- req:
		select {
		case err := <-req.result:
			resultChanPool.Put(req.result)
			return err
		case <-time.After(timeout):
			p.dumpGoroutines(id, timeout)
			return fmt.Errorf("submit task '%s' timeout after %v", id, timeout)
		}
	case <-time.After(timeout):
		p.dumpGoroutines(id, timeout)
		return fmt.Errorf("submit task '%s' timeout (send request) after %v", id, timeout)
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
	if p.stopSubmit != nil {
		p.stopSubmit()
		p.stopSubmit = nil
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
				atomic.LoadInt32(&p.ticktockDone) == 1 && atomic.LoadInt32(&p.submitDone) == 0 {
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
		atomic.StoreInt32(&p.submitDone, 0)
		p.goSubmit()
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

func (p *Pool) goSubmit() {
	p.submitCtx, p.stopSubmit = context.WithCancel(context.Background())
	for i := 0; i < p.submitDispatch; i++ {
		atomic.AddInt32(&p.submitDone, 1)
		go p.submit()
	}
}

func (p *Pool) submit() {
	defer func() {
		atomic.AddInt32(&p.submitDone, -1)
	}()
	for {
		select {
		case req := <-p.chSubmit:
			err := p.Submit(req.id, req.task)
			req.result <- err
		case <-p.submitCtx.Done():
			return
		}
	}
}

func (p *Pool) dumpGoroutines(id string, timeout time.Duration) {
	now := time.Now().Unix()
	last := atomic.LoadInt64(&p.dumplastTime)
	if now-last < int64(p.dumpMiniInterval.Seconds()) {
		return
	}
	if !atomic.CompareAndSwapInt64(&p.dumplastTime, last, now) {
		return
	}
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	logger.Log.Errorf("=== Goroutine Dump Start (taskId=%s,timeout:%v) ===\n%s\n=== Goroutine Dump End ===", id, timeout, buf[:n])
}
