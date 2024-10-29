package receive

import (
	"fmt"
	"io"

	"github.com/l-donovan/qcp/common"
	"golang.org/x/crypto/ssh"
)

func DownloadDirectory(client *ssh.Client, sourceDirectory, destDirectory string) error {
	// TODO: This shouldn't be hardcoded
	executable := "/home/ldonovan/bin/qcp"
	serveCmd := fmt.Sprintf("%s -d serve %s", executable, sourceDirectory)

	return common.RunWithOutput(client, serveCmd, func(stdout io.Reader) error {
		return ReceiveDirectory(destDirectory, stdout)
	})
}

func Download(client *ssh.Client, sourceFilePath, destFilePath string) error {
	// TODO: This shouldn't be hardcoded
	executable := "/home/ldonovan/bin/qcp"
	serveCmd := fmt.Sprintf("%s serve %s", executable, sourceFilePath)

	return common.RunWithOutput(client, serveCmd, func(stdout io.Reader) error {
		return Receive(destFilePath, stdout)
	})
}
