//go:generate stringer -type=EventType

package watch

const (
	None           EventType = 0x00
	All            EventType = 0xFF
	Create         EventType = 0x01
	Delete         EventType = 0x02
	Modify         EventType = 0x04
	Children       EventType = 0x08
	Connected      EventType = 0x10
	Disconnected   EventType = 0x20
	SessionExpired EventType = 0x40
)

type EventType uint32

type Event struct {
	Path string
	Type EventType
	User interface{}
}
