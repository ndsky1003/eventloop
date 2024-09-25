package operator

import "github.com/ndsky1003/task/itask"

type IOperator interface {
	HandleTask(task itask.ITask) error
}
