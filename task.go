package taskloop

import (
	"errors"
	"time"
)

type ITask interface {
	GetUpdateTime() time.Time
	GetCreateTime() time.Time
	SetUpdateTime(time.Time)
	SetCreateTime(time.Time)
	GetID() any // 唯一标识，大部分是主见
}

var ErrNoTask = errors.New("ErrNoTask")

type ITaskMgr interface {
	Add(ITask) error
	Next() (ITask, error)
	Remove(ID any) error
	UpdateStatus2Init(ID any) error
	HasNext() (bool, error)
}
