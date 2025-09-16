package serve

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/l-donovan/qcp/common"
	"github.com/l-donovan/qcp/protocol"
)

type UploadInfo struct {
	Filenames   []string
	Destination io.WriteCloser
	OffsetFile  string
	OffsetPos   int64

	foundOffsetFile bool
}

func (u *UploadInfo) addFileToTarArchive(tarWriter *tar.Writer, filePath string, directory string) error {
	archivePath := filePath

	// This check prevents us from getting rid of the file name/extension separator when it's
	// the only "." in the path.
	if directory != "." {
		archivePath = strings.Replace(filePath, directory, "", 1)
	}

	archivePath = strings.TrimPrefix(archivePath, string(filepath.Separator))
	archivePath = filepath.ToSlash(archivePath)

	if u.OffsetFile != "" && !u.foundOffsetFile {
		// We are looking for the partial file.

		if archivePath != u.OffsetFile {
			// This is not the partial file.
			return nil
		}

		// We found the partial file.
		u.foundOffsetFile = true
	}

	// Open the path to read from.
	fp, err := os.Open(filePath)

	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}

	defer func() {
		if err := fp.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing file: %v\n", err)
		}
	}()

	fileInfo, err := fp.Stat()

	if err != nil {
		return err
	}

	if u.OffsetFile != "" && archivePath == u.OffsetFile {
		fileInfo = common.NewPartialFileInfo(fileInfo, u.OffsetPos)

		if _, err := fp.Seek(u.OffsetPos, io.SeekStart); err != nil {
			return fmt.Errorf("seek partial file to offset: %w", err)
		}
	}

	// Create the path in the tarball.
	header, err := tar.FileInfoHeader(fileInfo, archivePath)

	if err != nil {
		return err
	}

	header.Name = archivePath

	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	// There isn't anything to copy for directories.
	if fileInfo.IsDir() {
		return nil
	}

	// Write the source file into the tarball at path from Create().
	if _, err = io.Copy(tarWriter, fp); err != nil {
		return err
	}

	return nil
}

// Serve sends file and directories as gzipped tarballs via stdout and a simple wire protocol.
func (u UploadInfo) Serve() error {
	if len(u.Filenames) == 0 {
		return errors.New("no filenames provided")
	}

	var flags byte

	if len(u.Filenames) == 1 {
		fileInfo, err := os.Stat(u.Filenames[0])

		if err != nil {
			return fmt.Errorf("stat %s: %w", u.Filenames[0], err)
		}

		if !fileInfo.IsDir() {
			flags |= protocol.ShouldUnpack
		}
	}

	if _, err := u.Destination.Write([]byte{flags}); err != nil {
		return fmt.Errorf("write flags: %w", err)
	}

	gzipWriter := gzip.NewWriter(u.Destination)

	defer func() {
		if err := gzipWriter.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error closing gzip writer: %v\n", err)
		}
	}()

	tarWriter := tar.NewWriter(gzipWriter)

	defer func() {
		if err := tarWriter.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error closing tar writer: %v\n", err)
		}
	}()

	for _, srcFilePath := range u.Filenames {
		info, err := os.Stat(srcFilePath)

		if err != nil {
			return err
		}

		basePath := path.Dir(srcFilePath)

		if err := u.addFileToTarArchive(tarWriter, srcFilePath, basePath); err != nil {
			return err
		}

		if info.IsDir() {
			if err := filepath.WalkDir(srcFilePath, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				// Skip the source directory root. This can be ignored because the source
				// directory root is just the root of the tarball.
				if path == srcFilePath {
					return nil
				}

				return u.addFileToTarArchive(tarWriter, path, basePath)
			}); err != nil {
				return err
			}
		}
	}

	return nil
}
