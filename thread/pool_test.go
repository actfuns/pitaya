package thread

import (
	"sync"
	"testing"
	"time"
)

func TestPool(t *testing.T) {
	pool, _ := NewPool(2, 0, 0)
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
	pool, _ := NewPool(5, 0, 0)
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
	pool, _ := NewPool(2, 0, 0)

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
