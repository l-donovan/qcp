package serve

import (
	"fmt"
	"io"

	"github.com/l-donovan/qcp/common"
	"golang.org/x/crypto/ssh"
)

func UploadDirectory(client *ssh.Client, srcDirectory, dstDirectory string, executable string) error {
	if executable == "" {
		foundExecutable, err := common.FindExecutable(client, "qcp")

		if err != nil {
			return err
		}

		executable = foundExecutable
	}

	serveCmd := fmt.Sprintf("%s receive -d %s", executable, dstDirectory)

	return common.RunWithPipes(client, serveCmd, func(stdin io.WriteCloser, stdout, stderr io.Reader) error {
		return ServeDirectory(srcDirectory, stdin)
	})
}

func UploadFile(client *ssh.Client, srcFilePath, dstFilePath string, executable string, compress bool) error {
	if executable == "" {
		foundExecutable, err := common.FindExecutable(client, "qcp")

		if err != nil {
			return err
		}

		executable = foundExecutable
	}

	serveCmd := fmt.Sprintf("%s receive %s", executable, dstFilePath)

	if !compress {
		serveCmd += " -u"
	}

	return common.RunWithPipes(client, serveCmd, func(stdin io.WriteCloser, stdout, stderr io.Reader) error {
		return ServeFile(srcFilePath, stdin, compress)
	})
}
