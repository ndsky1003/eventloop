package task

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/samber/lo"

	"github.com/ndsky1003/task/itask"
	"github.com/ndsky1003/task/operator"
	"github.com/ndsky1003/task/options"
	"github.com/ndsky1003/task/serialize"
	"github.com/ndsky1003/task/taskmgrstatus"
)

type handleStatusReq struct {
	status taskmgrstatus.T
	meta   any
}

type task_mgr struct {
	l                         sync.Mutex
	status                    uint32
	handling_order_task_types []uint32 // 正在执行的OrderTask的Type
	task_serialize            serialize.ITaskSerialize
	_handle_task_operator     operator.IOperator
	done                      chan struct{}
	concurrenceNum            chan struct{} // 限流
	opt                       *options.Option
}

/*
concurrenceNum:任务最大并发数量，数量必须大于强制顺序顺序类型的数量多，否则并发数有可能被其全部占用，而普通任务无法执行
sleep_deltas:任务处理错误的增量
task_serialize: 任务的管理，序列化、反序列化、hasnext，next等
fn:具体任务的处理逻辑
*/
func NewTaskMgr(task_serialize serialize.ITaskSerialize, op operator.IOperator, opts ...*options.Option) *task_mgr {
	if task_serialize == nil {
		panic("task_serialize must not nil")
	}

	if op == nil {
		panic("op must not nil")
	}
	opt := options.New().Merge(opts...)

	c := &task_mgr{
		concurrenceNum:        make(chan struct{}, opt.ConcurrenceNum),
		task_serialize:        task_serialize,
		_handle_task_operator: op,
		opt:                   opt,
	}
	if err := task_serialize.Init(); err != nil {
		panic(err)
	}
	c.Start()
	return c
}

func (this *task_mgr) push_handling_order_task_types(T uint32) {
	this.l.Lock()
	defer this.l.Unlock()
	if !lo.Contains(this.handling_order_task_types, T) {
		this.handling_order_task_types = append(this.handling_order_task_types, T)
	}
}

func (this *task_mgr) get_handling_order_task_types() []uint32 {
	this.l.Lock()
	defer this.l.Unlock()
	r := make([]uint32, len(this.handling_order_task_types))
	copy(r, this.handling_order_task_types)
	return r
}

func (this *task_mgr) pop_handling_order_task_types(T uint32) {
	this.l.Lock()
	defer this.l.Unlock()
	if index := lo.IndexOf(this.handling_order_task_types, T); index != -1 {
		this.handling_order_task_types = append(this.handling_order_task_types[:index], this.handling_order_task_types[index+1:]...)
	}
}

func (this *task_mgr) Start() {
	this.handleStatus(&handleStatusReq{status: taskmgrstatus.Start})
}

func (this *task_mgr) Stop() {
	this.handleStatus(&handleStatusReq{status: taskmgrstatus.Stop})
}

// 之所以加锁,是因为并发调用,关闭不应该关闭的done,那么每次给done加上一个指纹,关闭的时候必须凭借指纹关闭
func (this *task_mgr) handleStatus(req *handleStatusReq) {
	this.l.Lock()
	defer this.l.Unlock()
	switch req.status {
	case taskmgrstatus.Start, taskmgrstatus.HandleError:
		if req.status == taskmgrstatus.HandleError {
			if task, ok := req.meta.(itask.ITask); ok {
				if err := this.task_serialize.UpdateStatus2Init(task); err != nil {
					logger.Errf("task:%v,err:%v\n", task, err)
					break
				}
			}
		}
		if !atomic.CompareAndSwapUint32(&this.status, taskmgrstatus.Stop, taskmgrstatus.Start) {
			logger.Info("Loop has run")
		} else {
			if this.done != nil {
				close(this.done)
				this.done = nil
			}
			this.done = make(chan struct{}, 1)
			go this.run_loop(this.done)
		}
	case taskmgrstatus.Stop:
		if b, err := this.task_serialize.HasNext(this.handling_order_task_types...); err != nil {
			logger.Err(err)
		} else if !b {
			if !atomic.CompareAndSwapUint32(&this.status, taskmgrstatus.Start, taskmgrstatus.Stop) {
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
			if task, err := this.task_serialize.Next(this.get_handling_order_task_types()...); err == nil {
				this.concurrenceNum <- struct{}{}
				if task.IsOrder() {
					this.push_handling_order_task_types(task.GetType())
					go this.handdleTaskByType(task)
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
	err := this._handle_task_operator.HandleTask(task)
	if err != nil {
		time.AfterFunc(this.opt.NormalTaskHandleDelta, func() {
			this.handleStatus(&handleStatusReq{
				status: taskmgrstatus.HandleError,
				meta:   task,
			})
		})
	} else { // 处理成功，删除任务
		if err1 := this.task_serialize.Remove(task); err1 != nil {
			logger.Errf("task:%+v,err:%v", task, err1)
		}
	}
}

func (this *task_mgr) sleep(index uint8) uint8 {
	loopLength := len(this.opt.OrderTaskHandleDelta)
	si := int(index) % loopLength
	sv := this.opt.OrderTaskHandleDelta[si]
	// logger.Info("sleep:", sv)
	time.Sleep(sv)
	index++
	return index
}

// 某些任务按照添加顺序执行
func (this *task_mgr) handdleTaskByType(task itask.ITask) {
	t := task.GetType()
	logger.Info("handdleTaskByType:", t)
	var isPanic bool
	defer func() {
		if !isPanic {
			// 防止下次再次进入
			this.pop_handling_order_task_types(t)
		} else {
			logger.Infof("task type:%v not handle", t)
		}
		logger.Info("defer handdleTaskByType")
		<-this.concurrenceNum
	}()
	if isPanic = this._handdleTaskByType(task); isPanic {
		return
	}

here:
	task, err := this.task_serialize.NextByType(t)
	if err != nil {
		if err == itask.ErrNoTask {
			return
		}
		logger.Err("panic")
		goto here
	}
	if isPanic = this._handdleTaskByType(task); isPanic {
		return
	} else {
		goto here
	}
}

func (this *task_mgr) _handdleTaskByType(task itask.ITask) (isPanic bool) {
	var index uint8
here:
	err := this._handle_task_operator.HandleTask(task)
	if err != nil {
		index = this.sleep(index)
		if index == 255 {
			logger.Errf("isPanic,t:%v,err:%v", task.GetType(), err)
			isPanic = true
			return
		}
		goto here
	} else { // 处理成功，删除任务
		if err1 := this.task_serialize.Remove(task); err1 != nil {
			logger.Errf("task:%+v\n", task)
		}
	}
	return
}
