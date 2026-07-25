package osiris

import (
	"bufio"
	"fmt"
	"net"
	"time"
)

// tcpListener is an explicit owned loopback/network listener. It is kept
// separate from streams because accepting is not a read/write operation.
type tcpListener struct{ listener net.Listener }

func tcpListen(address string) (*tcpListener, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	return &tcpListener{listener: listener}, nil
}

func (l *tcpListener) address() string {
	if l == nil || l.listener == nil {
		return ""
	}
	return l.listener.Addr().String()
}

func (l *tcpListener) close() error {
	if l == nil || l.listener == nil {
		return nil
	}
	err := l.listener.Close()
	l.listener = nil
	return err
}

func (l *tcpListener) accept(timeoutMS int) (*streamValue, error) {
	if l == nil || l.listener == nil {
		return nil, fmt.Errorf("tcp listener is closed")
	}
	if timeoutMS < 0 {
		return nil, fmt.Errorf("tcp timeout must be non-negative")
	}
	if timeoutMS > 0 {
		if concrete, ok := l.listener.(*net.TCPListener); ok {
			_ = concrete.SetDeadline(time.Now().Add(time.Duration(timeoutMS) * time.Millisecond))
			defer concrete.SetDeadline(time.Time{})
		}
	}
	connection, err := l.listener.Accept()
	if err != nil {
		return nil, err
	}
	return tcpStream(connection), nil
}

func tcpDial(address string, timeoutMS int) (*streamValue, error) {
	if timeoutMS < 0 {
		return nil, fmt.Errorf("tcp timeout must be non-negative")
	}
	timeout := time.Duration(timeoutMS) * time.Millisecond
	connection, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, err
	}
	return tcpStream(connection), nil
}

func tcpStream(connection net.Conn) *streamValue {
	return &streamValue{id: connection.RemoteAddr().String(), path: connection.RemoteAddr().String(), mode: "tcp", conn: connection, reader: bufio.NewReader(connection), writer: bufio.NewWriter(connection)}
}
