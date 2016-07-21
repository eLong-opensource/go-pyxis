package node

import (
	"fmt"
)

const (
	Persist = iota
	Ephemeral
)

type Flag int

func (f Flag) String() string {
	switch f {
	case Persist:
		return "Persist"
	case Ephemeral:
		return "Ephemeral"
	default:
		return "???"
	}
}

type Node struct {
	Data     []byte
	Children []string
	Flags    Flag
	Version  uint64
	Owner    uint64
}

func (n *Node) String() string {
	return fmt.Sprintf("NChildren:%d\nData:%s\nChildren:%v\nFlags:%s\nVersion:%d\nOwner:%d",
		len(n.Children), string(n.Data), n.Children, n.Flags, n.Version, n.Owner)
}
