package sessions

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/l-donovan/qcp/common"
	"github.com/l-donovan/qcp/protocol"
	"golang.org/x/crypto/ssh"
)

type BrowseSession interface {
	EnterDirectory(name string) error
	ListContents() ([]common.ThinDirEntry, error)
	DownloadFile(name string, compress bool) (DownloadSession, error)
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
		return nil, fmt.Errorf("find executable: %w", err)
	}

	cmd, err := protocol.Parser.Marshal(executable, map[string]any{
		"mode":     "present",
		"location": location,
	})

	if err != nil {
		return nil, fmt.Errorf("generate command: %w", err)
	}

	session, err := common.Start(client, cmd)

	if err != nil {
		return nil, fmt.Errorf("start session: %w", err)
	}

	return &browseSession{session, location, client}, nil
}

func (s *browseSession) EnterDirectory(name string) error {
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
		return nil, fmt.Errorf("send list files command: %w", err)
	}

	// Get output
	result, err := srcReader.ReadString(protocol.EndTransmission)

	// We don't expect an EOF here, so we treat it as a normal error
	if err != nil {
		return nil, fmt.Errorf("read list files output: %w", err)
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
			return nil, fmt.Errorf("deserialize dir entry: %w", err)
		}

		entries = append(entries, *entry)
	}

	return entries, nil
}

func (s browseSession) DownloadFile(name string, compress bool) (DownloadSession, error) {
	srcFilePath := filepath.Join(s.path, name)

	return StartDownload(s.client, []string{srcFilePath}, compress)
}

func (s browseSession) Stop() {
	if _, err := s.Stdin.Write([]byte{protocol.Quit}); err != nil && err != io.EOF {
		_, _ = fmt.Fprintf(os.Stderr, "error when sending quit message to qcp process on remote host: %v\n", err)
	}

	// TODO: Yuck.
	s.Session.Session.Close()
}
