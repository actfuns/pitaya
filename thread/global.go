package thread

import (
	"context"
	"fmt"
	"math"
	"sync/atomic"
	"time"
)

const (
	// DefaultPoolSize is the default capacity for a default goroutine pool.
	DefaultPoolSize = math.MaxInt32

	// DefaultWorkerChanCap is the default capacity for a worker chan.
	DefaultWorkerChanCap = 1

	// DefaultCleanIntervalTime is the interval time to clean up goroutines.
	DefaultCleanIntervalTime = time.Second
)

const (
	OPENED = iota
	CLOSED
)

var (
	defaultAntsPool, _ = NewPool(DefaultPoolSize, DefaultWorkerChanCap, DefaultCleanIntervalTime)
	taskSeq            uint64
)

func Submit(ctx context.Context, taskId string, task func(ctx context.Context)) error {
	return defaultAntsPool.Submit(ctx, taskId, task)
}

func SubmitAnonymous(ctx context.Context, task func(ctx context.Context)) error {
	seq := atomic.AddUint64(&taskSeq, 1)
	return defaultAntsPool.Submit(ctx, fmt.Sprintf("pool:anonymous:%d_%d", time.Now().UnixMilli(), seq), task)
}

func Running() int {
	return defaultAntsPool.Running()
}

func Cap() int {
	return defaultAntsPool.Cap()
}

func Release() {
	defaultAntsPool.Release()
}

func Reboot() {
	defaultAntsPool.Reboot()
}
