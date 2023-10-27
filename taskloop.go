package task

import (
	"sync/atomic"
	"time"

	"github.com/ndsky1003/logger"
)

type HandleTaskFunc func(ITask) error

type task_mgr_status = uint32

const (
	task_mgr_status_stop         task_mgr_status = iota // 初始化
	task_mgr_status_start                               // 启动
	task_mgr_status_handle_error                        // 执行出错
)

const max_concurrence_num = 1000

type handle_status_chan struct {
	status task_mgr_status
	meta   any
}

type task_mgr struct {
	status             uint32
	task_serialize     ITaskSerialize
	_handle_task_fn    HandleTaskFunc
	done               chan struct{}
	handle_status_chan chan *handle_status_chan
	concurrenceNum     chan struct{} // 限流
}

/*
concurrenceNum:任务最大并发竖向
task_mgr: 任务的管理，序列化、反序列化、hasnext，next等
fn:具体任务的处理逻辑
*/

func NewTaskMgr(concurrenceNum uint32, task_serialize ITaskSerialize, fn HandleTaskFunc) *task_mgr {
	if concurrenceNum > max_concurrence_num {
		concurrenceNum = max_concurrence_num
	}
	if task_serialize == nil {
		panic("task_serialize must not nil")
	}

	if fn == nil {
		panic("fn must not nil")
	}

	c := &task_mgr{
		concurrenceNum:     make(chan struct{}, concurrenceNum),
		handle_status_chan: make(chan *handle_status_chan),
		task_serialize:     task_serialize,
		_handle_task_fn:    fn,
	}
	go c.handle_status()
	c.Start()
	return c
}

func (this *task_mgr) Start() {
	this.handle_status_chan <- &handle_status_chan{
		status: task_mgr_status_start,
	}
}

func (this *task_mgr) Stop() {
	this.handle_status_chan <- &handle_status_chan{
		status: task_mgr_status_stop,
	}
}

func (this *task_mgr) handle_status() {
	defer logger.Info("handle_status done")
	for v := range this.handle_status_chan {
		switch v.status {
		case task_mgr_status_start, task_mgr_status_handle_error:
			if v.status == task_mgr_status_handle_error {
				ID := v.meta
				if err := this.task_serialize.UpdateStatus2Init(ID); err != nil {
					logger.Errf("ID:%v,err:%v\n", ID, err)
					break
				}
			}
			if !atomic.CompareAndSwapUint32(&this.status, task_mgr_status_stop, task_mgr_status_start) {
				logger.Info("Loop has run")
			} else {
				if this.done != nil {
					close(this.done)
					this.done = nil
				}
				this.done = make(chan struct{}, 1)
				go this.RunLoop(this.done)
			}
		case task_mgr_status_stop:
			if b, err := this.task_serialize.HasNext(); err != nil {
				logger.Err(err)
			} else if !b {
				if !atomic.CompareAndSwapUint32(&this.status, task_mgr_status_start, task_mgr_status_stop) {
					logger.Info("Loop has stop")
				} else {
					if this.done != nil {
						close(this.done)
						this.done = nil
					}
				}
			}
		}
	}
}

// 添加任务
func (this *task_mgr) AddTask(task ITask) error {
	now := time.Now()
	if is_zero_time(task.GetUpdateTime()) {
		task.SetUpdateTime(now)
	}
	if is_zero_time(task.GetCreateTime()) {
		task.SetCreateTime(now)
	}
	err := this.task_serialize.Add(task)
	if err == nil {
		this.Start()
	}
	return err
}

func (this *task_mgr) RunLoop(done chan struct{}) {
	logger.Info("start runloop")
	defer func() {
		logger.Info("runloop done")
	}()
	for {
		select {
		case <-done:
			return
		default:
			if task, err := this.task_serialize.Next(); err == nil {
				this.concurrenceNum <- struct{}{}
				go this.handdleTask(task)
			} else {
				if err == ErrNoTask {
					this.Stop()
				} else {
					logger.Err(err)
				}
			}
		}
	}
}

func (this *task_mgr) handdleTask(task ITask) {
	defer func() {
		<-this.concurrenceNum
	}()
	err := this._handle_task_fn(task)
	ID := task.GetID()
	if err != nil {
		logger.Err("handdleTask:", err)
		this.handle_status_chan <- &handle_status_chan{
			status: task_mgr_status_handle_error,
			meta:   ID,
		}
	} else { // 处理成功，删除任务
		if err1 := this.task_serialize.Remove(ID); err1 != nil {
			logger.Errf("task:%+v\n", task)
		}
	}
}
