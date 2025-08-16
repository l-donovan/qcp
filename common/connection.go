package common

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	sshConfig "github.com/kevinburke/ssh_config"
	"golang.org/x/crypto/ssh"
)

type RunHandler func(stdin io.WriteCloser, stdout, stderr io.Reader) error

type ConnectionInfo struct {
	Username       string
	PrivateKeyPath string
	Hostname       string
	Port           int
}

func ParseConnectionString(connection string) (*ConnectionInfo, error) {
	connExpr := regexp.MustCompile(`(?:(.+)@)?([^:]+)(?::(\d+))?`)

	groups := connExpr.FindStringSubmatch(connection)

	if groups == nil {
		return nil, fmt.Errorf("could not parse connection string")
	}

	currentUser, err := user.Current()

	if err != nil {
		return nil, err
	}

	username := currentUser.Username
	port := 22
	hostname := groups[2]
	privateKeyPath := ""

	// Check config file for User
	if val := sshConfig.Get(hostname, "User"); val != "" {
		username = val
	}

	// The provided username takes precedence
	if groups[1] != "" {
		username = groups[1]
	}

	// Check config file for Port
	if val := sshConfig.Get(hostname, "Port"); val != "" {
		tempPort, err := strconv.Atoi(val)

		if err != nil {
			return nil, err
		}

		port = tempPort
	}

	// The provided port takes precedence
	if groups[3] != "" {
		tempPort, err := strconv.Atoi(groups[3])

		if err != nil {
			return nil, err
		}

		port = tempPort
	}

	// Check config file for IdentityFile(s)
	if vals := sshConfig.GetAll(hostname, "IdentityFile"); vals != nil {
		var path string

		val := vals[len(vals)-1]

		if val == "~" {
			path = currentUser.HomeDir
		} else if strings.HasPrefix(val, "~/") {
			path = filepath.Join(currentUser.HomeDir, val[2:])
		}

		privateKeyPath = path
	}

	// We do this last, otherwise config lookups by hostname might not work!
	if val := sshConfig.Get(hostname, "HostName"); val != "" {
		hostname = val
	}

	info := ConnectionInfo{
		Username:       username,
		PrivateKeyPath: privateKeyPath,
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

func FindExecutable(client *ssh.Client, name string) (string, error) {
	session, err := client.NewSession()

	if err != nil {
		return "", fmt.Errorf("create session: %v", err)
	}

	out, err := session.Output(fmt.Sprintf("$SHELL -l -c 'which %s'", name))

	if err != nil {
		return "", fmt.Errorf("which %s: %v", name, err)
	}

	return strings.TrimSpace(string(out)), nil
}

func LogErrors(stderr io.Reader) {
	stderrReader := bufio.NewReader(stderr)

	for {
		out, err := stderrReader.ReadString('\n')

		if err == io.EOF {
			break
		}

		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error when reading qcp output on remote host: %v\n", err)
			break
		}

		out = out[:len(out)-1]

		_, _ = fmt.Fprintf(os.Stderr, "remote error: %s\n", out)
	}
}

type Session struct {
	Session *ssh.Session
	Stdin   io.WriteCloser
	Stdout  io.Reader
	Stderr  io.Reader
}

func Start(client *ssh.Client, cmd string) (Session, error) {
	session, err := client.NewSession()

	if err != nil {
		return Session{}, fmt.Errorf("create session: %v", err)
	}

	stdin, err := session.StdinPipe()

	if err != nil {
		return Session{}, fmt.Errorf("get stdin pipe: %v", err)
	}

	stdout, err := session.StdoutPipe()

	if err != nil {
		return Session{}, fmt.Errorf("get stdout pipe: %v", err)
	}

	stderr, err := session.StderrPipe()

	if err != nil {
		return Session{}, fmt.Errorf("get stderr pipe: %v", err)
	}

	if err := session.Start(cmd); err != nil {
		return Session{}, fmt.Errorf("start command: %v", err)
	}

	return Session{session, stdin, stdout, stderr}, nil
}

func RunWithPipes(client *ssh.Client, cmd string, handle RunHandler) error {
	session, err := client.NewSession()

	if err != nil {
		return fmt.Errorf("create session: %v", err)
	}

	defer func() {
		if err := session.Close(); err != nil && err != io.EOF {
			fmt.Printf("error when closing session: %v\n", err)
		}
	}()

	stdin, err := session.StdinPipe()

	if err != nil {
		return fmt.Errorf("get stdin pipe: %v", err)
	}

	stdout, err := session.StdoutPipe()

	if err != nil {
		return fmt.Errorf("get stdout pipe: %v", err)
	}

	stderr, err := session.StderrPipe()

	if err != nil {
		return fmt.Errorf("get stderr pipe: %v", err)
	}

	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("start command: %v", err)
	}

	go LogErrors(stderr)

	if err := handle(stdin, stdout, stderr); err != nil {
		return fmt.Errorf("run handler: %v", err)
	}

	// We also check for EOF in case `handle` already closed stdin
	if err := stdin.Close(); err != nil && err != io.EOF {
		return fmt.Errorf("close stdin: %v", err)
	}

	if err := session.Wait(); err != nil {
		return fmt.Errorf("wait for command: %v", err)
	}

	return nil
}
