package thread

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestPool(t *testing.T) {
	pool, _ := NewPool(2, 0, 0, 10, 10)
	defer pool.Release()

	var wg sync.WaitGroup
	wg.Add(2)

	// 提交任务到协程池
	pool.Submit("1", func(id string) {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		t.Log("Task 1 executed")
	})

	pool.Submit("1", func(id string) {
		defer wg.Done()
		t.Log("Task 2 executed")
	})

	// 等待所有任务完成
	wg.Wait()
}

func TestPoolCapacity(t *testing.T) {
	pool, _ := NewPool(5, 0, 0, 10, 10)
	defer pool.Release()

	var wg sync.WaitGroup
	wg.Add(3)

	// 提交第一个任务
	pool.Submit("1", func(id string) {
		defer wg.Done()
		time.Sleep(2000 * time.Minute)
		t.Log("Task 1 executed")
	})

	// 提交第二个任务，应该等待第一个任务完成后执行
	pool.Submit("2", func(id string) {
		defer wg.Done()
		t.Log("Task 2 executed")
	})
	pool.Submit("3", func(id string) {
		defer wg.Done()
		t.Log("Task 3 executed")
	})

	// 等待所有任务完成
	wg.Wait()
}

func TestPoolRelease(t *testing.T) {
	pool, _ := NewPool(2, 0, 0, 10, 10)

	// 提交任务
	pool.Submit("1", func(id string) {
		time.Sleep(100 * time.Millisecond)
		t.Log("Task executed")
	})

	// 释放协程池
	pool.Release()

	// 尝试提交任务到已释放的协程池
	err := pool.Submit("2", func(id string) {
		t.Log("This should not be executed")
	})

	if err == nil {
		t.Error("Expected error when submitting task to released pool")
	}
}

func TestPoolTimeOut(t *testing.T) {
	// 创建一个 Pool，提交通道缓冲 10，派发1个提交协程
	p, err := NewPool(1, 0, time.Second, 1, 1)
	if err != nil {
		t.Fatalf("NewPool error: %v", err)
	}

	defer p.Release()

	taskID := "timeoutTask"

	// 提交一个会“卡住”的任务（模拟耗时超过timeout）
	longRunningTask := func(id string) {
		// 阻塞任务，超过timeout让SubmitWithTimeout超时返回
		fmt.Printf("11111111111\n")
		time.Sleep(time.Second * 1)
	}

	// 设置超时时间为 500ms，明显小于任务运行时间
	timeout := 500 * time.Millisecond
	for range 10 {
		if err := p.SubmitWithTimeout(taskID, timeout, longRunningTask); err != nil {
			fmt.Printf("expected timeout error, got nil\n")
		}
	}

	time.Sleep(time.Second * 180)
}
