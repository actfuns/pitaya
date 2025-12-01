package service

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/topfreegames/pitaya/v2/logger"
	"github.com/topfreegames/pitaya/v2/thread"
)

type TaskService struct {
	taskSeq uint64
	pool    *thread.Pool
}

func NewTaskService(size int, workerChanCap int, expiryDurationSecond int) (*TaskService, error) {
	pool, err := thread.NewPool(size, int32(workerChanCap), time.Duration(expiryDurationSecond)*time.Second)
	if err != nil {
		return nil, err
	}
	return &TaskService{
		pool: pool,
	}, nil
}

func (ts *TaskService) Submit(ctx context.Context, id string, task func(context.Context)) error {
	return ts.pool.Submit(ctx, id, task)
}

func (ts *TaskService) SubmitWithTimeout(ctx context.Context, id string, timeout time.Duration, task func(context.Context)) error {
	return ts.pool.SubmitWithTimeout(ctx, id, timeout, task)
}

func (ts *TaskService) SubmitAnonymous(ctx context.Context, task func(context.Context)) error {
	seq := atomic.AddUint64(&ts.taskSeq, 1)
	return ts.pool.Submit(ctx, fmt.Sprintf("pitaya:task:anonymous:%d_%d", time.Now().UnixMilli(), seq), task)
}

func (ts *TaskService) Shutdown() {
	if err := ts.pool.ReleaseTimeout(time.Second * 30); err != nil {
		logger.Log.Errorf("task service shutdown error:%v", err)
	}
	logger.Log.Infof("taskService stopped!")
}
