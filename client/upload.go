package client

import (
	"fmt"
	"github.com/l-donovan/qcp/server"
	"golang.org/x/crypto/ssh"
	"io"
)

func UploadDirectory(client *ssh.Client, sourceDirectory, destDirectory string) error {
	// TODO: This shouldn't be hardcoded
	executable := "/home/ldonovan/bin/qcp"
	serveCmd := fmt.Sprintf("%s -d receive %s", executable, destDirectory)

	return RunWithInput(client, serveCmd, func(stdin io.Writer) error {
		return server.ServeDirectory(sourceDirectory, stdin)
	})
}

func Upload(client *ssh.Client, sourceFilePath, destFilePath string) error {
	// TODO: This shouldn't be hardcoded
	executable := "/home/ldonovan/bin/qcp"
	serveCmd := fmt.Sprintf("%s receive %s", executable, destFilePath)

	return RunWithInput(client, serveCmd, func(stdin io.Writer) error {
		return server.Serve(sourceFilePath, stdin)
	})
}
