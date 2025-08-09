package receive

import (
	"fmt"
	"io"

	"github.com/l-donovan/qcp/common"
	"golang.org/x/crypto/ssh"
)

func DownloadDirectory(client *ssh.Client, srcDirectory, dstDirectory string, executable string) error {
	if executable == "" {
		foundExecutable, err := common.FindExecutable(client, "qcp")

		if err != nil {
			return err
		}

		executable = foundExecutable
	}

	serveCmd := fmt.Sprintf("%s serve -d %s", executable, srcDirectory)

	return common.RunWithPipes(client, serveCmd, func(stdin io.WriteCloser, stdout, stderr io.Reader) error {
		return ReceiveDirectory(dstDirectory, stdout, fmt.Printf)
	})
}

func Download(client *ssh.Client, srcFilePath, dstFilePath string, executable string) error {
	if executable == "" {
		foundExecutable, err := common.FindExecutable(client, "qcp")

		if err != nil {
			return err
		}

		executable = foundExecutable
	}

	serveCmd := fmt.Sprintf("%s serve %s", executable, srcFilePath)

	return common.RunWithPipes(client, serveCmd, func(stdin io.WriteCloser, stdout, stderr io.Reader) error {
		return Receive(dstFilePath, stdout)
	})
}
