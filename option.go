package pyxis

// ReadOption 为读取操作的一些配置
type ReadOption struct {
	// 是否显示隐藏子节点
	ShowHidden bool
}

// WriteOption 为写入操作的一些配置
type WriteOption struct {
	// for Create
	IsEphemeral bool
}
