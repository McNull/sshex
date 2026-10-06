package service

import (
	"errors"
	"fmt"
	"net"
	"syscall"
)

func localPortInUse(port int) (bool, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			return true, nil
		}
		return false, err
	}
	_ = ln.Close()
	return false, nil
}
