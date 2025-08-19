package sessions

import (
	"bufio"
	"fmt"
	"github.com/l-donovan/qcp/common"
	"github.com/l-donovan/qcp/protocol"
	"github.com/l-donovan/qcp/serve"
	"golang.org/x/crypto/ssh"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type BrowseSession interface {
	EnterDirectory(name string) error
	ListContents() ([]common.ThinDirEntry, error)
	SelectFile(name string) (serve.DownloadInfo, error)
	Stop()
}

type browseSession struct {
	common.Session
	path   string
	client *ssh.Client
}

func Browse(client *ssh.Client, location string) (BrowseSession, error) {
	executable, err := common.FindExecutable(client, "qcp")

	if err != nil {
		return nil, fmt.Errorf("find executable: %v", err)
	}

	cmd := fmt.Sprintf("%s present %s", executable, location)
	session, err := common.Start(client, cmd)

	if err != nil {
		return nil, fmt.Errorf("start session: %v", err)
	}

	return browseSession{session, location, client}, nil
}

func (s browseSession) EnterDirectory(name string) error {
	if _, err := fmt.Fprintf(s.Stdin, "%c%s%c", protocol.Enter, name, protocol.EndTransmission); err != nil {
		return err
	}

	s.path = filepath.Join(s.path, name)

	return nil
}

func (s browseSession) ListContents() ([]common.ThinDirEntry, error) {
	srcReader := bufio.NewReader(s.Stdout)

	// List files
	if _, err := s.Stdin.Write([]byte{protocol.ListFiles}); err != nil {
		return nil, fmt.Errorf("send list files command: %v", err)
	}

	// Get output
	result, err := srcReader.ReadString(protocol.EndTransmission)

	// We don't expect an EOF here, so we treat it as a normal error
	if err != nil {
		return nil, fmt.Errorf("read list files output: %v", err)
	}

	var entries []common.ThinDirEntry
	serializedEntries := strings.Split(strings.TrimSuffix(result, string(protocol.EndTransmission)), string(protocol.FileSeparator))

	for _, rawEntry := range serializedEntries {
		// This happens in empty directories because strings.Split("", "<separator>") returns []string{""}, not []string{}
		if rawEntry == "" {
			continue
		}

		entry, err := common.DeserializeDirEntry(rawEntry)

		if err != nil {
			return nil, fmt.Errorf("deserialize dir entry: %v", err)
		}

		entries = append(entries, *entry)
	}

	return entries, nil
}

func (s browseSession) SelectFile(name string) (serve.DownloadInfo, error) {
	srcFilePath := filepath.Join(s.path, name)

	// TODO: Parameterize `compress`.
	session, err := StartDownload(s.client, srcFilePath, true)

	if err != nil {
		return serve.DownloadInfo{}, fmt.Errorf("start download %s: %v", srcFilePath, err)
	}

	defer session.Stop()

	return session.GetDownloadInfo(name)
}

func (s browseSession) Stop() {
	if _, err := s.Stdin.Write([]byte{protocol.Quit}); err != nil && err != io.EOF {
		_, _ = fmt.Fprintf(os.Stderr, "error when sending quit message to qcp process on remote host: %v\n", err)
	}

	// TODO: Yuck.
	s.Session.Session.Close()
}
