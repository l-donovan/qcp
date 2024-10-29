package receive

import (
	"fmt"
	"io"

	"github.com/l-donovan/qcp/common"
	"golang.org/x/crypto/ssh"
)

func DownloadDirectory(client *ssh.Client, sourceDirectory, destDirectory string) error {
	executable, err := common.FindExecutable(client, "qcp")

	if err != nil {
		return err
	}

	serveCmd := fmt.Sprintf("%s -d serve %s", executable, sourceDirectory)

	return common.RunWithOutput(client, serveCmd, func(stdout io.Reader) error {
		return ReceiveDirectory(destDirectory, stdout)
	})
}

func Download(client *ssh.Client, sourceFilePath, destFilePath string) error {
	executable, err := common.FindExecutable(client, "qcp")

	if err != nil {
		return err
	}

	serveCmd := fmt.Sprintf("%s serve %s", executable, sourceFilePath)

	return common.RunWithOutput(client, serveCmd, func(stdout io.Reader) error {
		return Receive(destFilePath, stdout)
	})
}
