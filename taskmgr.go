package task

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/ndsky1003/logger"
	"github.com/samber/lo"

	"github.com/ndsky1003/task/itask"
	"github.com/ndsky1003/task/options"
)

type HandleTaskFunc func(itask.ITask) error

type task_mgr_status = uint32

const (
	task_mgr_status_stop         task_mgr_status = iota // 初始化
	task_mgr_status_start                               // 启动
	task_mgr_status_handle_error                        // 执行出错
)

type handleStatusReq struct {
	status task_mgr_status
	meta   any
}

type task_mgr struct {
	l                         sync.Mutex
	status                    uint32
	handling_order_task_types []uint32 // 正在执行的OrderTask的Type
	task_serialize            itask.ITaskSerialize
	_handle_task_fn           HandleTaskFunc
	done                      chan struct{}
	concurrenceNum            chan struct{} // 限流
	opt                       *options.MgrOptions
}

/*
concurrenceNum:任务最大并发数量，数量必须大于强制顺序顺序类型的数量多，否则并发数有可能被其全部占用，而普通任务无法执行
sleep_deltas:任务处理错误的增量
task_serialize: 任务的管理，序列化、反序列化、hasnext，next等
fn:具体任务的处理逻辑
*/
func NewTaskMgr(task_serialize itask.ITaskSerialize, fn HandleTaskFunc, opts ...*options.MgrOptions) *task_mgr {
	if task_serialize == nil {
		panic("task_serialize must not nil")
	}

	if fn == nil {
		panic("fn must not nil")
	}
	opt := options.Mgr()
	opt.Merge(opts...)

	c := &task_mgr{
		concurrenceNum:  make(chan struct{}, opt.ConcurrenceNum),
		task_serialize:  task_serialize,
		_handle_task_fn: fn,
		opt:             opt,
	}
	if err := task_serialize.Init(); err != nil {
		panic(err)
	}
	c.Start()
	return c
}

func (this *task_mgr) push_handling_asce_task_types(T uint32) {
	this.l.Lock()
	defer this.l.Unlock()
	if !lo.Contains(this.handling_order_task_types, T) {
		this.handling_order_task_types = append(this.handling_order_task_types, T)
	}
}

func (this *task_mgr) get_handling_asce_task_types() []uint32 {
	this.l.Lock()
	defer this.l.Unlock()
	r := make([]uint32, len(this.handling_order_task_types))
	copy(r, this.handling_order_task_types)
	return r
}

func (this *task_mgr) pop_handling_asce_task_types(T uint32) {
	this.l.Lock()
	defer this.l.Unlock()
	index := lo.IndexOf(this.handling_order_task_types, T)
	this.handling_order_task_types = lo.Drop(this.handling_order_task_types, index)
}

func (this *task_mgr) Start() {
	this.handleStatus(&handleStatusReq{status: task_mgr_status_start})
}

func (this *task_mgr) Stop() {
	this.handleStatus(&handleStatusReq{status: task_mgr_status_stop})
}

func (this *task_mgr) handleStatus(req *handleStatusReq) {
	this.l.Lock()
	defer this.l.Unlock()
	switch req.status {
	case task_mgr_status_start, task_mgr_status_handle_error:
		if req.status == task_mgr_status_handle_error {
			if task, ok := req.meta.(itask.ITask); ok {
				if err := this.task_serialize.UpdateStatus2Init(task); err != nil {
					logger.Errf("task:%v,err:%v\n", task, err)
					break
				}
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
			go this.run_loop(this.done)
		}
	case task_mgr_status_stop:
		if b, err := this.task_serialize.HasNext(this.get_handling_asce_task_types()...); err != nil {
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

// 添加任务
func (this *task_mgr) Add(task itask.ITask) error {
	return this.add(task)
}

func (this *task_mgr) add(task itask.ITask) error {
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

func (this *task_mgr) run_loop(done chan struct{}) {
	logger.Info("start runloop")
	defer func() {
		logger.Info("runloop done")
	}()
	for {
		select {
		case <-done:
			return
		default:
			if task, err := this.task_serialize.Next(this.get_handling_asce_task_types()...); err == nil {
				this.concurrenceNum <- struct{}{}
				if task.IsOrder() {
					this.push_handling_asce_task_types(task.GetType())
					go this.handdleTaskByType(task.GetType())
				} else {
					go this.handdleTask(task)
				}
			} else {
				if err == itask.ErrNoTask {
					this.Stop()
				} else {
					logger.Err(err)
				}
			}
		}
	}
}

func (this *task_mgr) handdleTask(task itask.ITask) {
	defer func() {
		<-this.concurrenceNum
	}()
	err := this._handle_task_fn(task)
	if err != nil {
		logger.Err("handdleTask:", err)
		time.AfterFunc(this.opt.NormalTaskHandleDelta, func() {
			this.handleStatus(&handleStatusReq{
				status: task_mgr_status_handle_error,
				meta:   task,
			})
		})
	} else { // 处理成功，删除任务
		if err1 := this.task_serialize.Remove(task); err1 != nil {
			logger.Errf("task:%+v\n", task)
		}
	}
}

func (this *task_mgr) sleep(index uint8) uint8 {
	loopLength := len(this.opt.OrderTaskHandleDelta)
	sv := int(index) % loopLength
	time.Sleep(this.opt.OrderTaskHandleDelta[sv])
	index++
	return index
}

// 某些任务按照添加顺序执行
func (this *task_mgr) handdleTaskByType(t uint32) {
	var isPanic bool
	defer func() {
		if !isPanic {
			// 防止下次再次进入
			this.pop_handling_asce_task_types(t)
		}
		<-this.concurrenceNum
	}()
	var index uint8

here:
	task, err := this.task_serialize.NextByType(t)
	if err != nil {
		logger.Err("handdleTask:", err)
		index = this.sleep(index)
		if index == 255 {
			logger.Errf("isPanic,t:%v,err:%v", t, err)
			isPanic = true
			return
		}
		goto here
	}

here1:
	err = this._handle_task_fn(task)
	if err != nil {
		index = this.sleep(index)
		if index == 255 {
			logger.Errf("isPanic,t:%v,err:%v", t, err)
			isPanic = true
			return
		}
		goto here1
	} else { // 处理成功，删除任务
		if err1 := this.task_serialize.Remove(task); err1 != nil {
			logger.Errf("task:%+v\n", task)
		}
	}
}
