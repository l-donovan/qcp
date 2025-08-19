package serve

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/l-donovan/qcp/protocol"
)

type BrowseInfo struct {
	Location    string
	Source      io.Reader
	Destination io.WriteCloser
}

func serializeDirEntry(entry os.DirEntry) (string, error) {
	info, err := entry.Info()

	if err != nil {
		return "", err
	}

	fileMode := uint32(info.Mode())

	return fmt.Sprintf("%s%c%d", entry.Name(), protocol.GroupSeparator, fileMode), nil
}

func (b BrowseInfo) Present() error {
	srcReader := bufio.NewReader(b.Source)

	defer func() {
		if err := b.Destination.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error when closing write end: %v\n", err)
		}
	}()

	for {
		command, err := srcReader.ReadByte()

		if err == io.EOF {
			return nil
		}

		if err != nil {
			return err
		}

		switch command {
		case protocol.ListFiles:
			var items []string
			entries, err := os.ReadDir(b.Location)

			if err != nil {
				return err
			}

			for _, entry := range entries {
				serializedEntry, err := serializeDirEntry(entry)

				if err != nil {
					return err
				}

				items = append(items, serializedEntry)
			}

			itemStr := strings.Join(items, string(protocol.FileSeparator))

			if _, err := fmt.Fprintf(b.Destination, "%s%c", itemStr, protocol.EndTransmission); err != nil {
				return err
			}
		case protocol.Enter:
			result, err := srcReader.ReadString(protocol.EndTransmission)

			if err != nil {
				return err
			}

			entryName := strings.TrimSuffix(result, string(protocol.EndTransmission))
			b.Location = path.Join(b.Location, entryName)
		case protocol.Quit:
			return nil
		}
	}
}
