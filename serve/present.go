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

func serializeDirEntry(entry os.DirEntry) (string, error) {
	info, err := entry.Info()

	if err != nil {
		return "", err
	}

	fileMode := uint32(info.Mode())

	return fmt.Sprintf("%s%c%d", entry.Name(), protocol.GroupSeparator, fileMode), nil
}

func Present(location string, src io.Reader, dst io.WriteCloser) error {
	srcReader := bufio.NewReader(src)

	defer func() {
		if err := dst.Close(); err != nil {
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
			entries, err := os.ReadDir(location)

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

			if _, err := fmt.Fprintf(dst, "%s%c", itemStr, protocol.EndTransmission); err != nil {
				return err
			}
		case protocol.Select:
			result, err := srcReader.ReadString(protocol.EndTransmission)

			if err != nil {
				return err
			}

			entryName := strings.TrimSuffix(result, string(protocol.EndTransmission))
			location = path.Join(location, entryName)
			info, err := os.Stat(location)

			if err != nil {
				return err
			}

			if info.IsDir() {
				return ServeDirectory(location, dst)
			} else {
				return Serve(location, dst)
			}
		case protocol.Enter:
			result, err := srcReader.ReadString(protocol.EndTransmission)

			if err != nil {
				return err
			}

			entryName := strings.TrimSuffix(result, string(protocol.EndTransmission))
			location = path.Join(location, entryName)
		case protocol.Quit:
			return nil
		}
	}
}
