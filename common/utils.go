package common

import (
	"fmt"
	"net"
	"path"
	"strings"
)

func CreateIdentifier(names []string) string {
	basenames := make([]string, len(names))

	for i, name := range names {
		basenames[i] = path.Base(name)
	}

	id := strings.Join(basenames, "__")
	maxLen := 50
	andMore := " (...)"

	if len(id) > maxLen {
		id = id[:(maxLen-len(andMore))] + andMore
	}

	return id
}

// GetOutboundIP gets the preferred outbound IP address of this machine.
func GetOutboundIP() (net.IP, error) {
	conn, err := net.Dial("udp", "1.1.1.1:1")

	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP, nil
}
