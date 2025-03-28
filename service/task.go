package service

import (
	"context"
	"time"

	"github.com/topfreegames/pitaya/v2/constants"
	"github.com/topfreegames/pitaya/v2/logger"
	"github.com/topfreegames/pitaya/v2/thread"
)

type TaskService struct {
	pool *thread.Pool
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

func (c *TaskService) Submit(ctx context.Context, id string, task func(context.Context)) error {
	taskIdVal := ctx.Value(constants.TaskIDKey)
	if taskIdVal != nil && taskIdVal.(string) == id {
		logger.WithCtx(ctx).Warnf("task %s already submitted", id)
		thread.RunSafe(func() { task(ctx) })
		return nil
	}
	return c.pool.Submit(id, func(s string) {
		ctx = context.WithValue(ctx, constants.TaskIDKey, s)
		task(ctx)
	})
}

func (c *TaskService) Shutdown() {
	if err := c.pool.ReleaseTimeout(time.Second * 30); err != nil {
		logger.Log.Errorf("task service shutdown error:%v", err)
	}
}
