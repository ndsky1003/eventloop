package itask

import (
	"errors"
	"time"
)

/*这是一个模版,一下几个是内置字段
type Task struct {
	ID         primitive.ObjectID `bson:"ID"`
	Type       uint32             `bson:"Type"`
	UpdateTime time.Time          `bson:"UpdateTime"`
	CreateTime time.Time          `bson:"CreateTime"`
	Status     taskstatus.T       `bson:"Status"`
	Order      bool               `bson:"Order"`
}

func (this *Task) GetID() any {
	return this.ID
}

func (this *Task) GetUpdateTime() time.Time {
	return this.UpdateTime
}

func (this *Task) GetCreateTime() time.Time {
	return this.CreateTime
}

func (this *Task) SetUpdateTime(a time.Time) {
	this.UpdateTime = a
}

func (this *Task) SetCreateTime(a time.Time) {
	this.CreateTime = a
}

func (this *Task) GetType() uint32 {
	return this.Type
}

func (this *Task) IsOrder() bool {
	return this.Order
}

*/

type ITask interface {
	GetID() any // 唯一标识，大部分是主键
	GetUpdateTime() time.Time
	GetCreateTime() time.Time
	SetUpdateTime(time.Time)
	SetCreateTime(time.Time)
	GetType() uint32 // 标识任务类型，某一类的任务，需要按照添加顺序执行,比如充值任务
	IsOrder() bool   // 有序
}

var ErrNoTask = errors.New("ErrNoTask")

type ITaskSerialize interface {
	// 初始化，用于将异常关闭的任务，状态重置等作用
	Init() error
	// 添加任务
	Add(ITask) error
	// 下一个任务,按照UpdateTime顺序获取task，排除某一类任务，有些任务需要按照顺序执行
	// 先添加的先获取，获取后更新UpdateTime，防止一个任务一直获取，阻塞死任务
	Next(exclude_t ...uint32) (ITask, error)
	// 下一个任务，按照CreateTime顺序获取task
	// 同一类任务，先添加的先获取，自然有阻塞死的情况，mgr里面处理
	NextByType(t uint32) (ITask, error)
	// 删除任务，任务执行完毕需要删除
	Remove(ITask) error
	// 任务状态还原。eg:如果任务出错，需要将状态还原，等待下次执行
	UpdateStatus2Init(ITask) error
	// 某一类有类型有顺序限制的
	HasNext(exclude_t ...uint32) (bool, error)
}
