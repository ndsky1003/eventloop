package taskmgrstatus

type T = uint32

const (
	Stop        T = iota //初始化
	Start                //启动
	HandleError          //执行出错
)
