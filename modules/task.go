package modules

import (
	"time"

	"github.com/topfreegames/pitaya/v2/modules/pool"
)

type Task struct {
	Base
	timeout int32
	pool    *pool.Pool
}

func NewTask(size int, workerChanCap int32, expiryDuration time.Duration, timeout int32) (*Task, error) {
	pool, err := pool.NewPool(size, workerChanCap, expiryDuration)
	if err != nil {
		return nil, err
	}
	return &Task{
		pool:    pool,
		timeout: timeout,
	}, nil
}

func (c *Task) Shutdown() error {
	return c.pool.ReleaseTimeout(time.Duration(c.timeout) * time.Second)
}
