package serve

import (
	"fmt"
	"io"

	"github.com/l-donovan/qcp/common"
	"golang.org/x/crypto/ssh"
)

func UploadDirectory(client *ssh.Client, srcDirectory, dstDirectory string) error {
	executable, err := common.FindExecutable(client, "qcp")

	if err != nil {
		return err
	}

	serveCmd := fmt.Sprintf("%s receive -d %s", executable, dstDirectory)

	return common.RunWithPipes(client, serveCmd, func(stdin io.WriteCloser, stdout, stderr io.Reader) error {
		go common.LogErrors(stderr)
		return ServeDirectory(srcDirectory, stdin)
	})
}

func Upload(client *ssh.Client, srcFilePath, dstFilePath string) error {
	executable, err := common.FindExecutable(client, "qcp")

	if err != nil {
		return err
	}

	serveCmd := fmt.Sprintf("%s receive %s", executable, dstFilePath)

	return common.RunWithPipes(client, serveCmd, func(stdin io.WriteCloser, stdout, stderr io.Reader) error {
		go common.LogErrors(stderr)
		return Serve(srcFilePath, stdin)
	})
}
