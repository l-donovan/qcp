package client

import (
	"fmt"
	"github.com/l-donovan/qcp/server"
	"golang.org/x/crypto/ssh"
	"io"
)

func DownloadDirectory(client *ssh.Client, sourceDirectory, destDirectory string) error {
	// TODO: This shouldn't be hardcoded
	executable := "/home/ldonovan/bin/qcp"
	serveCmd := fmt.Sprintf("%s -d serve %s", executable, sourceDirectory)

	return RunWithOutput(client, serveCmd, func(stdout io.Reader) error {
		return server.ReceiveDirectory(destDirectory, stdout)
	})
}

func Download(client *ssh.Client, sourceFilePath, destFilePath string) error {
	// TODO: This shouldn't be hardcoded
	executable := "/home/ldonovan/bin/qcp"
	serveCmd := fmt.Sprintf("%s serve %s", executable, sourceFilePath)

	return RunWithOutput(client, serveCmd, func(stdout io.Reader) error {
		return server.Receive(destFilePath, stdout)
	})
}
