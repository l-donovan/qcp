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

	if groups[1] != "" {
		username = groups[1]
	}

	if groups[3] != "" {
		tempPort, err := strconv.Atoi(groups[3])

		if err != nil {
			return nil, err
		}

		port = tempPort
	}

	if val := sshConfig.Get(hostname, "HostName"); val != "" {
		hostname = val
	}

	if val := sshConfig.Get(hostname, "Port"); val != "" {
		tempPort, err := strconv.Atoi(val)

		if err != nil {
			return nil, err
		}

		port = tempPort
	}

	if val := sshConfig.Get(hostname, "IdentityFile"); val != "" {
		var path string

		if val == "~" {
			path = currentUser.HomeDir
		} else if strings.HasPrefix(val, "~/") {
			path = filepath.Join(currentUser.HomeDir, val[2:])
		}

		privateKeyPath = path
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
		return "", err
	}

	out, err := session.Output(fmt.Sprintf("which %s", name))

	if err != nil {
		return "", err
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

		_, _ = fmt.Fprintf(os.Stderr, "error when running qcp command on remote host: %s\n", out)
	}
}

func RunWithPipes(client *ssh.Client, cmd string, handle RunHandler) error {
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

	err = handle(stdin, stdout, stderr)

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
