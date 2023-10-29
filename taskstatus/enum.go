package taskstatus

type T = uint8

const (
	Init     T = iota // 初始化
	Handling          // 处理中
	Handled           // 处理完成，一般不存在，因为处理完成直接删除
)
