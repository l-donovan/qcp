package client

import (
	"bufio"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
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

func RunWithInput(client *ssh.Client, cmd string, handler func(stdin io.Writer) error) error {
	session, err := client.NewSession()

	if err != nil {
		return err
	}

	defer func() {
		if err := session.Close(); err != nil && err != io.EOF {
			fmt.Printf("error when closing session: %v\n", err)
		}
	}()

	stdin, err := session.StdinPipe()

	if err != nil {
		return err
	}

	stderr, err := session.StderrPipe()

	if err != nil {
		return err
	}

	err = session.Start(cmd)

	if err != nil {
		return err
	}

	go func(stderr io.Reader) {
		stderrReader := bufio.NewReader(stderr)

		for {
			out, err := stderrReader.ReadString('\n')

			if err == io.EOF {
				break
			}

			if err != nil {
				fmt.Printf("error when reading: %v\n", err)
				break
			}

			fmt.Printf("got stderr: %s\n", out)
		}
	}(stderr)

	err = handler(stdin)

	if err != nil {
		return err
	}

	err = stdin.Close()

	if err != nil {
		return err
	}

	err = session.Wait()

	if err != nil {
		return err
	}

	return nil
}

func RunWithOutput(client *ssh.Client, cmd string, handler func(stdout io.Reader) error) error {
	session, err := client.NewSession()

	if err != nil {
		return err
	}

	defer func() {
		if err := session.Close(); err != nil && err != io.EOF {
			fmt.Printf("error when closing session: %v\n", err)
		}
	}()

	stdout, err := session.StdoutPipe()

	if err != nil {
		return err
	}

	stderr, err := session.StderrPipe()

	if err != nil {
		return err
	}

	err = session.Start(cmd)

	if err != nil {
		return err
	}

	go func(stderr io.Reader) {
		stderrReader := bufio.NewReader(stderr)

		for {
			out, err := stderrReader.ReadString('\n')

			if err == io.EOF {
				break
			}

			if err != nil {
				fmt.Printf("error when reading: %v\n", err)
				break
			}

			fmt.Printf("got stderr: %s\n", out)
		}
	}(stderr)

	err = handler(stdout)

	if err != nil {
		return err
	}

	err = session.Wait()

	if err != nil {
		return err
	}

	return nil
}
