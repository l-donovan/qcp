package client

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	"net"
	"os"
	"regexp"
	"strconv"
)

type ConnectionInfo struct {
	Username       string
	PrivateKeyPath string
	Hostname       string
	Port           int
}

func ParseConnectionString(connection string, defaultUser string, defaultPort int) (*ConnectionInfo, error) {
	connExpr := regexp.MustCompile(`(?:(.+)@)?([^:]+)(?::(\d+))?`)

	groups := connExpr.FindStringSubmatch(connection)

	if groups == nil {
		return nil, fmt.Errorf("could not parse connection string")
	}

	user := defaultUser
	port := defaultPort
	hostname := groups[2]

	if groups[1] != "" {
		user = groups[1]
	}

	if groups[3] != "" {
		tempPort, err := strconv.Atoi(groups[3])

		if err != nil {
			return nil, err
		}

		port = tempPort
	}

	info := ConnectionInfo{
		Username:       user,
		PrivateKeyPath: "",
		Hostname:       hostname,
		Port:           port,
	}

	return &info, nil
}

func CreateClient(info ConnectionInfo) (*ssh.Client, error) {
	privateKeyBytes, err := os.ReadFile(info.PrivateKeyPath)

	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(privateKeyBytes)

	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User: info.Username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			// use OpenSSH's known_hosts file if you care about host validation
			return nil
		},
	}

	connectionString := fmt.Sprintf("%s:%d", info.Hostname, info.Port)
	client, err := ssh.Dial("tcp", connectionString, config)

	if err != nil {
		return nil, err
	}

	return client, nil
}
