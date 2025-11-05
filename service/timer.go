package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/topfreegames/pitaya/v2/constants"
	"github.com/topfreegames/pitaya/v2/logger"
	"github.com/topfreegames/pitaya/v2/timer"
)

type timerEntity struct {
	taskid string
	fn     func(context.Context)
}

type TimerService struct {
	id          uint64
	taskService *TaskService
	timingWheel *timer.TimingWheel
	timerIds    sync.Map
	maxRetries  int
}

func NewTimerService(interval time.Duration, numSlots int, maxRetries int, taskService *TaskService) (*TimerService, error) {
	ts := TimerService{
		id:          0,
		taskService: taskService,
	}
	timingWheel, err := timer.NewTimingWheelWithTicker(interval, numSlots, ts.execute, timer.NewTicker(interval))
	if err != nil {
		return nil, err
	}
	ts.maxRetries = maxRetries
	ts.timingWheel = timingWheel
	return &ts, nil
}

func (ts *TimerService) SetInterval(taskid string, delay time.Duration, counter int, fn func(context.Context)) (uint64, error) {
	if counter == 0 {
		return 0, timer.ErrArgument
	}
	var timerId uint64
	for i := 0; i < ts.maxRetries; i++ {
		id := atomic.AddUint64(&ts.id, 1)
		if id == 0 {
			continue
		}
		if _, exists := ts.timerIds.Load(id); exists {
			continue
		}
		if _, loaded := ts.timerIds.LoadOrStore(id, struct{}{}); loaded {
			continue
		}
		timerId = id
		break
	}
	if timerId == 0 {
		return 0, timer.ErrMaxRetries
	}
	if err := ts.timingWheel.SetTimer(timerId, &timerEntity{
		taskid: taskid,
		fn:     fn,
	}, delay, int(counter)); err != nil {
		ts.timerIds.Delete(timerId)
		return 0, err
	}
	return timerId, nil
}

func (ts *TimerService) ClearInterval(timerId uint64) error {
	if err := ts.timingWheel.RemoveTimer(timerId); err != nil {
		return err
	}
	ts.timerIds.Delete(timerId)
	return nil
}

func (ts *TimerService) execute(key, value any) {
	task := value.(*timerEntity)
	ctx := context.WithValue(context.Background(), constants.TimerIdKey, key)
	if err := ts.taskService.Submit(ctx, task.taskid, task.fn); err != nil {
		logger.Log.Errorf("timerService task %s execute error: %v", task.taskid, err)
	}
}

func (ts *TimerService) Shutdown() {
	ts.timingWheel.Drain(func(key, value any) {})
	logger.Log.Infof("timerService stopped!")
}
