package receive

import (
	"fmt"
	"io"

	"github.com/l-donovan/qcp/common"
	"golang.org/x/crypto/ssh"
)

func DownloadDirectory(client *ssh.Client, srcDirectory, dstDirectory string) error {
	executable, err := common.FindExecutable(client, "qcp")

	if err != nil {
		return err
	}

	serveCmd := fmt.Sprintf("%s serve -d %s", executable, srcDirectory)

	return common.RunWithPipes(client, serveCmd, func(stdin io.WriteCloser, stdout, stderr io.Reader) error {
		go common.LogErrors(stderr)
		return ReceiveDirectory(dstDirectory, stdout)
	})
}

func Download(client *ssh.Client, srcFilePath, dstFilePath string) error {
	executable, err := common.FindExecutable(client, "qcp")

	if err != nil {
		return err
	}

	serveCmd := fmt.Sprintf("%s serve %s", executable, srcFilePath)

	return common.RunWithPipes(client, serveCmd, func(stdin io.WriteCloser, stdout, stderr io.Reader) error {
		go common.LogErrors(stderr)
		return Receive(dstFilePath, stdout)
	})
}
