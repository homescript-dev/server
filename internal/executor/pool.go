package executor

import (
	"homescript-server/internal/logger"
	"homescript-server/internal/types"
	"sync"
)

// Task represents a script execution task
type Task struct {
	ScriptPath string
	Event      *types.Event
}

// Pool manages a pool of workers for executing Lua scripts
type Pool struct {
	executor  *Executor
	workers   int
	taskQueue chan Task
	wg        sync.WaitGroup
	stopOnce  sync.Once
	stopChan  chan struct{}
}

// NewPool creates a new worker pool
func NewPool(executor *Executor, workers int, queueSize int) *Pool {
	return &Pool{
		executor:  executor,
		workers:   workers,
		taskQueue: make(chan Task, queueSize),
		stopChan:  make(chan struct{}),
	}
}

// Start begins processing tasks
func (p *Pool) Start() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	logger.Debug("Started %d workers", p.workers)
}

// Submit adds a task to the queue
func (p *Pool) Submit(task Task) {
	select {
	case p.taskQueue <- task:
		// Task queued successfully
	case <-p.stopChan:
		logger.Warn("Pool is stopped, task rejected")
	default:
		logger.Warn("Task queue full, dropping task for script: %s", task.ScriptPath)
	}
}

// Stop gracefully shuts down the pool
func (p *Pool) Stop() {
	p.stopOnce.Do(func() {
		close(p.stopChan)
		close(p.taskQueue)
		p.wg.Wait()
		logger.Debug("Worker pool stopped")
	})
}

func (p *Pool) worker(id int) {
	defer p.wg.Done()

	for {
		select {
		case task, ok := <-p.taskQueue:
			if !ok {
				logger.Debug("Worker %d: task queue closed", id)
				return
			}

			logger.Debug("Worker %d: executing %s", id, task.ScriptPath)
			if err := p.executor.Execute(task.ScriptPath, task.Event); err != nil {
				logger.Error("Worker %d: script error in %s: %v", id, task.ScriptPath, err)
			}

		case <-p.stopChan:
			logger.Debug("Worker %d: stopping", id)
			return
		}
	}
}
