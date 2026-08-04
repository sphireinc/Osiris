package osiris

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// tcpListener is an explicit owned loopback/network listener. It is kept
// separate from streams because accepting is not a read/write operation.
type tcpListener struct {
	mu       sync.Mutex
	listener net.Listener
}

// SocketConfig controls the portable subset of TCP behavior. Nil booleans
// retain the documented default; non-nil values are explicit overrides. OS
// specific controls belong in capability-checked adapters, not this contract.
type SocketConfig struct {
	NoDelay          *bool
	KeepAlive        *bool
	KeepAlivePeriod  time.Duration
	ConnectTimeout   time.Duration
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	ReadBufferBytes  int
	WriteBufferBytes int
	WriteBatchBytes  int
}

func socketBool(value bool) *bool { return &value }

// DefaultSocketConfig favors interactive latency while retaining Go's portable
// TCP behavior. Deadlines are intentionally opt-in: task/session contexts
// remain the cancellation contract for unbounded operations.
func DefaultSocketConfig() SocketConfig {
	return SocketConfig{
		NoDelay:         socketBool(true),
		KeepAlive:       socketBool(true),
		WriteBatchBytes: 4096,
	}
}

func (config SocketConfig) validate() error {
	if config.KeepAlivePeriod < 0 {
		return fmt.Errorf("tcp keepalive period must be non-negative")
	}
	if config.ConnectTimeout < 0 {
		return fmt.Errorf("tcp connect timeout must be non-negative")
	}
	if config.ReadTimeout < 0 {
		return fmt.Errorf("tcp read timeout must be non-negative")
	}
	if config.WriteTimeout < 0 {
		return fmt.Errorf("tcp write timeout must be non-negative")
	}
	if config.ReadBufferBytes < 0 {
		return fmt.Errorf("tcp read buffer must be non-negative")
	}
	if config.WriteBufferBytes < 0 {
		return fmt.Errorf("tcp write buffer must be non-negative")
	}
	if config.WriteBatchBytes < 0 {
		return fmt.Errorf("tcp write batch must be non-negative")
	}
	return nil
}

func (config SocketConfig) apply(connection *net.TCPConn) error {
	if err := config.validate(); err != nil {
		return err
	}
	return config.applyValidated(connection)
}

func (config SocketConfig) applyValidated(connection *net.TCPConn) error {
	if config.NoDelay != nil {
		if err := connection.SetNoDelay(*config.NoDelay); err != nil {
			return err
		}
	}
	if config.KeepAlive != nil {
		if err := connection.SetKeepAlive(*config.KeepAlive); err != nil {
			return err
		}
	}
	if config.KeepAlivePeriod > 0 {
		if err := connection.SetKeepAlivePeriod(config.KeepAlivePeriod); err != nil {
			return err
		}
	}
	if config.ReadBufferBytes > 0 {
		if err := connection.SetReadBuffer(config.ReadBufferBytes); err != nil {
			return err
		}
	}
	if config.WriteBufferBytes > 0 {
		if err := connection.SetWriteBuffer(config.WriteBufferBytes); err != nil {
			return err
		}
	}
	if config.ReadTimeout > 0 {
		if err := connection.SetReadDeadline(time.Now().Add(config.ReadTimeout)); err != nil {
			return err
		}
	}
	if config.WriteTimeout > 0 {
		if err := connection.SetWriteDeadline(time.Now().Add(config.WriteTimeout)); err != nil {
			return err
		}
	}
	return nil
}

func tcpListen(address string) (*tcpListener, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	return &tcpListener{listener: listener}, nil
}

func (l *tcpListener) address() string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.listener == nil {
		return ""
	}
	return l.listener.Addr().String()
}

func (l *tcpListener) close() error {
	_, err := l.closeOnce()
	return err
}

func (l *tcpListener) closeOnce() (bool, error) {
	if l == nil {
		return false, nil
	}
	l.mu.Lock()
	listener := l.listener
	l.listener = nil
	l.mu.Unlock()
	if listener == nil {
		return false, nil
	}
	return true, listener.Close()
}

func (l *tcpListener) accept(timeoutMS int) (*streamValue, error) {
	return l.acceptContext(context.Background(), timeoutMS)
}

func (l *tcpListener) acceptContext(ctx context.Context, timeoutMS int) (*streamValue, error) {
	return l.acceptContextWithConfig(ctx, timeoutMS, DefaultSocketConfig())
}

// acceptContextWithConfig accepts one connection and applies config before the
// stream becomes visible. It keeps bulk-transfer policy explicit for internal
// callers such as benchmarks while tcp_accept retains its portable defaults.
func (l *tcpListener) acceptContextWithConfig(ctx context.Context, timeoutMS int, config SocketConfig) (*streamValue, error) {
	if timeoutMS < 0 {
		return nil, fmt.Errorf("tcp timeout must be non-negative")
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	listener := l.listener
	l.mu.Unlock()
	if listener == nil {
		return nil, fmt.Errorf("tcp listener is closed")
	}
	if concrete, ok := listener.(*net.TCPListener); ok {
		deadline := tcpDeadline(ctx, timeoutMS)
		if !deadline.IsZero() {
			if err := concrete.SetDeadline(deadline); err != nil {
				return nil, err
			}
			defer concrete.SetDeadline(time.Time{})
		}
		return acceptWithCancellation(ctx, concrete, config)
	}
	connection, err := listener.Accept()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return tcpStreamWithValidatedConfig(connection, config)
}

func tcpDial(address string, timeoutMS int) (*streamValue, error) {
	return tcpDialContext(context.Background(), address, timeoutMS)
}

func tcpDialContext(ctx context.Context, address string, timeoutMS int) (*streamValue, error) {
	config := DefaultSocketConfig()
	if timeoutMS > 0 {
		config.ConnectTimeout = time.Duration(timeoutMS) * time.Millisecond
	}
	return tcpDialWithConfig(ctx, address, config)
}

func tcpDialWithConfig(ctx context.Context, address string, config SocketConfig) (*streamValue, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dialer := net.Dialer{}
	if config.ConnectTimeout > 0 {
		dialer.Timeout = config.ConnectTimeout
	}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	return tcpStreamWithValidatedConfig(connection, config)
}

func tcpStream(connection net.Conn) (*streamValue, error) {
	return tcpStreamWithConfig(connection, DefaultSocketConfig())
}

func tcpStreamWithConfig(connection net.Conn, config SocketConfig) (*streamValue, error) {
	if err := config.validate(); err != nil {
		_ = connection.Close()
		return nil, err
	}
	return tcpStreamWithValidatedConfig(connection, config)
}

func tcpStreamWithValidatedConfig(connection net.Conn, config SocketConfig) (*streamValue, error) {
	if concrete, ok := connection.(*net.TCPConn); ok {
		if err := config.applyValidated(concrete); err != nil {
			_ = connection.Close()
			return nil, err
		}
	}
	batch := config.WriteBatchBytes
	if batch == 0 {
		batch = 4096
	}
	address := connection.RemoteAddr().String()
	return &streamValue{id: address, path: address, mode: "tcp", conn: connection, writeBatch: batch}, nil
}

func tcpListenValue(args []Value) (Value, error) {
	return tcpListenContextValue(context.Background(), args)
}

func tcpListenContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("tcp_listen expects 1 argument")
	}
	address, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	if err := ctx.Err(); err != nil {
		return nullValue(), err
	}
	listener, err := tcpListen(address)
	if err != nil {
		return nullValue(), err
	}
	return tcpListenerValueOf(listener), nil
}

func tcpAcceptValue(args []Value) (Value, error) {
	return tcpAcceptContextValue(context.Background(), args)
}

func tcpAcceptContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nullValue(), fmt.Errorf("tcp_accept expects listener and optional timeout")
	}
	listener, err := args[0].AsTCPListener()
	if err != nil {
		return nullValue(), err
	}
	timeout := 0
	if len(args) == 2 {
		timeout, err = args[1].Int()
		if err != nil {
			return nullValue(), err
		}
	}
	stream, err := listener.acceptContext(ctx, timeout)
	if err != nil {
		return nullValue(), err
	}
	return streamValueOf(stream), nil
}

func tcpDialValue(args []Value) (Value, error) {
	return tcpDialContextValue(context.Background(), args)
}

func tcpDialContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nullValue(), fmt.Errorf("tcp_dial expects address and optional timeout")
	}
	address, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	timeout := 0
	if len(args) == 2 {
		timeout, err = args[1].Int()
		if err != nil {
			return nullValue(), err
		}
	}
	stream, err := tcpDialContext(ctx, address, timeout)
	if err != nil {
		return nullValue(), err
	}
	return streamValueOf(stream), nil
}

func closeTCPListenerValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("close expects 1 argument")
	}
	listener, err := args[0].AsTCPListener()
	if err != nil {
		return nullValue(), err
	}
	closed, err := listener.closeOnce()
	if err != nil {
		return nullValue(), err
	}
	if !closed {
		return boolValue(false), nil
	}
	return boolValue(true), nil
}

func tcpDeadline(ctx context.Context, timeoutMS int) time.Time {
	var deadline time.Time
	if timeoutMS > 0 {
		deadline = time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
	}
	if contextDeadline, ok := ctx.Deadline(); ok && (deadline.IsZero() || contextDeadline.Before(deadline)) {
		deadline = contextDeadline
	}
	return deadline
}

func acceptWithCancellation(ctx context.Context, listener *net.TCPListener, config SocketConfig) (*streamValue, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var stop func() bool
	if ctx.Done() != nil {
		stop = context.AfterFunc(ctx, func() { _ = listener.SetDeadline(time.Now()) })
	}
	connection, err := listener.Accept()
	if stop != nil {
		stop()
	}
	if err != nil {
		if contextErr := socketContextError(ctx, err); contextErr != nil {
			return nil, contextErr
		}
		return nil, err
	}
	if contextErr := ctx.Err(); contextErr != nil {
		_ = connection.Close()
		return nil, contextErr
	}
	return tcpStreamWithValidatedConfig(connection, config)
}

func socketContextError(ctx context.Context, err error) error {
	if ctx == nil {
		return nil
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	var networkErr net.Error
	if !errors.As(err, &networkErr) || !networkErr.Timeout() {
		return nil
	}
	if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return nil
}
