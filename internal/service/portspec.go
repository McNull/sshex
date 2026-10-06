package service

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mcnull/sshex/internal/errs"
)

const defaultTargetHost = "localhost"

type PortSpec struct {
	LocalPort  int
	TargetHost string
	TargetPort int
}

func ParsePortSpec(spec string) (PortSpec, error) {
	parts := strings.Split(spec, ":")
	if len(parts) == 0 || len(parts) > 3 || parts[0] == "" {
		return PortSpec{}, fmt.Errorf("%w: %q", errs.ErrInvalidInput, spec)
	}

	localPort, err := parsePort(parts[0])
	if err != nil {
		return PortSpec{}, err
	}

	result := PortSpec{
		LocalPort:  localPort,
		TargetHost: defaultTargetHost,
		TargetPort: localPort,
	}

	if len(parts) >= 2 && parts[1] != "" {
		result.TargetHost = parts[1]
	}
	if len(parts) == 3 && parts[2] != "" {
		targetPort, err := parsePort(parts[2])
		if err != nil {
			return PortSpec{}, err
		}
		result.TargetPort = targetPort
	}

	return result, nil
}

func parsePort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%w: invalid port %q", errs.ErrInvalidInput, value)
	}
	return port, nil
}
