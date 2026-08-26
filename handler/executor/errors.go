package executor

import "errors"

var (
	// ErrInvalidConfig 表示 executor 配置非法。
	ErrInvalidConfig = errors.New("gnalloy/handler/executor: invalid config")
	// ErrMissingExecutor 表示 offload Handler 缺少执行器。
	ErrMissingExecutor = errors.New("gnalloy/handler/executor: missing executor")
	// ErrMissingHandler 表示 offload Handler 缺少被代理的业务 Handler。
	ErrMissingHandler = errors.New("gnalloy/handler/executor: missing handler")
	// ErrClosedExecutor 表示执行器已经关闭。
	ErrClosedExecutor = errors.New("gnalloy/handler/executor: executor closed")
	// ErrTaskQueueFull 表示执行器任务队列已满。
	ErrTaskQueueFull = errors.New("gnalloy/handler/executor: task queue full")
	// ErrHandlerPanic 表示业务 Handler 执行时触发 panic。
	ErrHandlerPanic = errors.New("gnalloy/handler/executor: handler panic")
)
