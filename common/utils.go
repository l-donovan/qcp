package common

import (
	"os"

	"golang.org/x/crypto/ssh/terminal"
)

func Getch() (byte, error) {
	stdinFd := int(os.Stdin.Fd())
	oldState, err := terminal.MakeRaw(stdinFd)

	if err != nil {
		return 0, err
	}

	defer func() {
		terminal.Restore(stdinFd, oldState)
	}()

	b := make([]byte, 1)
	_, err = os.Stdin.Read(b)

	if err != nil {
		return 0, err
	}

	return b[0], nil
}
