package serialize

import "github.com/ndsky1003/task/itask"

type ITaskSerialize interface {
	// 初始化，用于将异常关闭的任务，状态重置等作用
	Init() error
	// 添加任务
	Add(itask.ITask) error
	// 下一个任务,按照UpdateTime顺序获取task，排除某一类任务，有些任务需要按照顺序执行
	// 先添加的先获取，获取后更新UpdateTime，防止一个任务一直获取，阻塞死任务
	Next(exclude_t ...uint32) (itask.ITask, error)
	// 下一个任务，按照CreateTime顺序获取task
	// 同一类任务，先添加的先获取，自然有阻塞死的情况，mgr里面处理
	NextByType(t uint32) (itask.ITask, error)
	// 删除任务，任务执行完毕需要删除
	Remove(itask.ITask) error
	// 任务状态还原。eg:如果任务出错，需要将状态还原，等待下次执行
	UpdateStatus2Init(itask.ITask) error
	// 某一类有类型有顺序限制的
	HasNext(exclude_t ...uint32) (bool, error)
}
