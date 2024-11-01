package receive

import (
	"fmt"
	"github.com/l-donovan/qcp/common"
	"golang.org/x/crypto/ssh"
	"io"
)

func Pick(client *ssh.Client, location string, executable string) error {
	if executable == "" {
		foundExecutable, err := common.FindExecutable(client, "qcp")

		if err != nil {
			return err
		}

		executable = foundExecutable
	}

	serveCmd := fmt.Sprintf("%s present %s", executable, location)

	return common.RunWithPipes(client, serveCmd, func(stdin io.WriteCloser, stdout, stderr io.Reader) error {
		go common.LogErrors(stderr)
		return pick(stdin, stdout)
	})
}
