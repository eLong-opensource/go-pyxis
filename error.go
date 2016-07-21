package pyxis

import (
	"errors"

	"github.com/eLong-INF/go-pyxis/rpc"
)

var (
	ErrClosed          = errors.New("use of closed client")
	ErrConnecting      = errors.New("client is connecting")
	ErrNotFound        = errors.New("node not found")
	ErrNotLeader       = errors.New("not leader")
	ErrSessionExpired  = errors.New("session expired")
	ErrInvalidArgument = errors.New("invalid argument")
	ErrAgain           = errors.New("try again")
	ErrExists          = errors.New("node exists")
	ErrInternal        = errors.New("internal error")
	ErrUnknown         = errors.New("unknown error")
)

func newRPCError(e *rpc.Error) error {
	switch e.GetType() {
	case rpc.ErrorType_kNotFound:
		return ErrNotFound
	case rpc.ErrorType_kNotLeader:
		return ErrNotLeader
	case rpc.ErrorType_kSessionExpired:
		return ErrSessionExpired
	case rpc.ErrorType_kInvalidArgument:
		return ErrInvalidArgument
	case rpc.ErrorType_kAgain:
		return ErrAgain
	case rpc.ErrorType_kExist:
		return ErrExists
	case rpc.ErrorType_kInternal:
		return ErrInternal
	default:
		return ErrUnknown
	}
}
