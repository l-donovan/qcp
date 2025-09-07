package serve

import (
	"archive/tar"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/l-donovan/qcp/protocol"
)

func addFileToTarArchive(tarWriter *tar.Writer, filePath string, directory string) error {
	archivePath := filePath

	// This check prevents us from getting rid of the file name/extension separator when it's
	// the only "." in the path.
	if directory != "." {
		archivePath = strings.Replace(filePath, directory, "", 1)
	}

	archivePath = strings.TrimPrefix(archivePath, string(filepath.Separator))
	archivePath = filepath.ToSlash(archivePath)

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

type UploadInfo struct {
	Filenames   []string
	Destination io.WriteCloser
	Directory   bool
	Compressed  bool
}

// serveDirectory sends a directory via stdout and a simple wire protocol.
// The directory is added to a tar archive and compressed with gzip.
func (u UploadInfo) serveDirectory() error {
	if _, err := u.Destination.Write([]byte{protocol.IsDirectory}); err != nil {
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

	if err := filepath.WalkDir(u.Filenames[0], func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the source directory root. This can be ignored because the source
		// directory root is just the root of the tarball.
		if path == u.Filenames[0] {
			return nil
		}

		return addFileToTarArchive(tarWriter, path, u.Filenames[0])
	}); err != nil {
		return err
	}

	return nil
}

func (u UploadInfo) serveFileCompressed() error {
	fileModeBytes := make([]byte, 4)

	fp, err := os.Open(u.Filenames[0])

	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}

	defer func() {
		if err := fp.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing file: %v\n", err)
		}
	}()

	fileInfo, err := fp.Stat()

	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	// One uint32 containing flags.
	if _, err := u.Destination.Write([]byte{protocol.IsCompressed}); err != nil {
		return fmt.Errorf("write flags: %w", err)
	}

	// One uint32 containing the file mode.
	fileMode := uint32(fileInfo.Mode())
	binary.LittleEndian.PutUint32(fileModeBytes, fileMode)

	if _, err := u.Destination.Write(fileModeBytes); err != nil {
		return fmt.Errorf("write file mode: %w", err)
	}

	gzipWriter := gzip.NewWriter(u.Destination)

	defer func() {
		if err := gzipWriter.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing gzip writer: %v\n", err)
		}
	}()

	// The file contents.
	if _, err := io.Copy(gzipWriter, fp); err != nil {
		return fmt.Errorf("copy source file: %w", err)
	}

	return nil
}

func (u UploadInfo) serveFileUncompressed() error {
	fileSizeBytes := make([]byte, 4)
	fileModeBytes := make([]byte, 4)

	fp, err := os.Open(u.Filenames[0])

	if err != nil {
		return fmt.Errorf("open file for reading: %w", err)
	}

	defer func() {
		if err := fp.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing file: %v\n", err)
		}
	}()

	fileInfo, err := fp.Stat()

	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	// One uint32 containing flags.
	if _, err := u.Destination.Write([]byte{0}); err != nil {
		return fmt.Errorf("write flags: %w", err)
	}

	// One uint32 containing the file size.
	fileSize := uint32(fileInfo.Size())
	binary.LittleEndian.PutUint32(fileSizeBytes, fileSize)

	if _, err := u.Destination.Write(fileSizeBytes); err != nil {
		return fmt.Errorf("write file size: %w", err)
	}

	// One uint32 containing the file mode.
	fileMode := uint32(fileInfo.Mode())
	binary.LittleEndian.PutUint32(fileModeBytes, fileMode)

	if _, err := u.Destination.Write(fileModeBytes); err != nil {
		return fmt.Errorf("write file mode: %w", err)
	}

	// The file contents.
	if _, err := io.Copy(u.Destination, fp); err != nil {
		return err
	}

	return nil
}

func (u UploadInfo) serveMultipleFiles() error {
	if _, err := u.Destination.Write([]byte{protocol.IsDirectory}); err != nil {
		return fmt.Errorf("write flags: %w", err)
	}

	gzipWriter := gzip.NewWriter(u.Destination)

	defer func() {
		if err := gzipWriter.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing gzip writer: %v\n", err)
		}
	}()

	tarWriter := tar.NewWriter(gzipWriter)

	defer func() {
		if err := tarWriter.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing tar writer: %v\n", err)
		}
	}()

	for _, srcFilePath := range u.Filenames {
		info, err := os.Stat(srcFilePath)

		if err != nil {
			return err
		}

		basePath := path.Dir(srcFilePath)

		if err := addFileToTarArchive(tarWriter, srcFilePath, basePath); err != nil {
			return err
		}

		if info.IsDir() {
			if err := filepath.WalkDir(srcFilePath, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				if path == srcFilePath {
					return nil
				}

				return addFileToTarArchive(tarWriter, path, basePath)
			}); err != nil {
				return err
			}
		}
	}

	return nil
}

// Serve sends file and directories via stdout and a simple wire protocol.
func (u UploadInfo) Serve() error {
	if len(u.Filenames) == 0 {
		return errors.New("no filenames provided")
	}

	if len(u.Filenames) > 1 {
		return u.serveMultipleFiles()
	} else if u.Directory {
		return u.serveDirectory()
	} else if u.Compressed {
		return u.serveFileCompressed()
	} else {
		return u.serveFileUncompressed()
	}
}
