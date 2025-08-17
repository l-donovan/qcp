package serve

import (
	"archive/tar"
	"compress/gzip"
	"encoding/binary"
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
	archivePath := strings.Replace(filePath, directory, "", 1)
	archivePath = strings.TrimPrefix(archivePath, string(filepath.Separator))
	archivePath = filepath.ToSlash(archivePath)

	// Open the path to read from.
	fp, err := os.Open(filePath)

	if err != nil {
		return fmt.Errorf("open file: %v", err)
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

// ServeDirectory sends a directory via stdout and a simple wire protocol.
// The directory is added to a tar archive and compressed with gzip.
func ServeDirectory(srcDirectory string, dst io.WriteCloser) error {
	if _, err := dst.Write([]byte{protocol.IsDirectory}); err != nil {
		return fmt.Errorf("write flags: %v", err)
	}

	gzipWriter := gzip.NewWriter(dst)

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

	if err := filepath.WalkDir(srcDirectory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the source directory root. This can be ignored because the source
		// directory root is just the root of the tarball.
		if path == srcDirectory {
			return nil
		}

		return addFileToTarArchive(tarWriter, path, srcDirectory)
	}); err != nil {
		return err
	}

	return nil
}

func ServeMultipleFiles(srcFilePaths []string, dst io.WriteCloser) error {
	if _, err := dst.Write([]byte{protocol.IsDirectory}); err != nil {
		return fmt.Errorf("write flags: %v", err)
	}

	gzipWriter := gzip.NewWriter(dst)

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

	for _, srcFilePath := range srcFilePaths {
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

func serveFileCompressed(srcFilePath string, dst io.WriteCloser) error {
	fileModeBytes := make([]byte, 4)

	fp, err := os.Open(srcFilePath)

	if err != nil {
		return fmt.Errorf("open source file: %v", err)
	}

	defer func() {
		if err := fp.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing file: %v\n", err)
		}
	}()

	fileInfo, err := fp.Stat()

	if err != nil {
		return fmt.Errorf("stat file: %v", err)
	}

	// One uint32 containing flags.
	if _, err := dst.Write([]byte{protocol.IsCompressed}); err != nil {
		return fmt.Errorf("write flags: %v", err)
	}

	// One uint32 containing the file mode.
	fileMode := uint32(fileInfo.Mode())
	binary.LittleEndian.PutUint32(fileModeBytes, fileMode)

	if _, err := dst.Write(fileModeBytes); err != nil {
		return fmt.Errorf("write file mode: %v", err)
	}

	gzipWriter := gzip.NewWriter(dst)

	defer func() {
		if err := gzipWriter.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing gzip writer: %v\n", err)
		}
	}()

	// The file contents.
	if _, err := io.Copy(gzipWriter, fp); err != nil {
		return fmt.Errorf("copy source file: %v", err)
	}

	return nil
}

func serveFileUncompressed(srcFilePath string, dst io.WriteCloser) error {
	fileSizeBytes := make([]byte, 4)
	fileModeBytes := make([]byte, 4)

	fp, err := os.Open(srcFilePath)

	if err != nil {
		return fmt.Errorf("open file for reading: %v", err)
	}

	defer func() {
		if err := fp.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing file: %v\n", err)
		}
	}()

	fileInfo, err := fp.Stat()

	if err != nil {
		return fmt.Errorf("stat file: %v", err)
	}

	// One uint32 containing flags.
	if _, err := dst.Write([]byte{0}); err != nil {
		return fmt.Errorf("write flags: %v", err)
	}

	// One uint32 containing the file size.
	fileSize := uint32(fileInfo.Size())
	binary.LittleEndian.PutUint32(fileSizeBytes, fileSize)

	if _, err := dst.Write(fileSizeBytes); err != nil {
		return fmt.Errorf("write file size: %v", err)
	}

	// One uint32 containing the file mode.
	fileMode := uint32(fileInfo.Mode())
	binary.LittleEndian.PutUint32(fileModeBytes, fileMode)

	if _, err := dst.Write(fileModeBytes); err != nil {
		return fmt.Errorf("write file mode: %v", err)
	}

	// The file contents.
	if _, err := io.Copy(dst, fp); err != nil {
		return err
	}

	return nil
}

// Serve sends a file via stdout and a simple wire protocol.
func ServeFile(srcFilePath string, dst io.WriteCloser, compress bool) error {
	if compress {
		return serveFileCompressed(srcFilePath, dst)
	} else {
		return serveFileUncompressed(srcFilePath, dst)
	}
}
