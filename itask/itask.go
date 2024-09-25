package itask

import (
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
