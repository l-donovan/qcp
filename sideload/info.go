package sideload

import (
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

const (
	OsLinux   = "linux"
	OsMac     = "darwin"
	OsUnknown = "unknown"

	ArchAmd64   = "amd64"
	ArchUnknown = "unknown"
)

func getRawOs(client *ssh.Client) (string, error) {
	session, err := client.NewSession()

	if err != nil {
		return "", err
	}

	out, err := session.Output("uname")

	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

func getRawArch(client *ssh.Client) (string, error) {
	session, err := client.NewSession()

	if err != nil {
		return "", err
	}

	out, err := session.Output("uname -m")

	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

func getOs(client *ssh.Client) (string, error) {
	rawOs, err := getRawOs(client)

	if err != nil {
		return "", err
	}

	if strings.HasPrefix(strings.ToLower(rawOs), "linux") {
		return OsLinux, nil
	}

	if strings.HasPrefix(strings.ToLower(rawOs), "darwin") {
		return OsMac, nil
	}

	return OsUnknown, fmt.Errorf("unknown operating system %s", rawOs)
}

func getArch(client *ssh.Client) (string, error) {
	rawArch, err := getRawArch(client)

	if err != nil {
		return "", err
	}

	switch rawArch {
	case "x86_64":
		return ArchAmd64, nil
	}

	return ArchUnknown, fmt.Errorf("unknown architecture %s", rawArch)
}
