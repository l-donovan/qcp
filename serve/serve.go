package serve

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func AddFileToTarArchive(tarWriter *tar.Writer, filePath string, directory string) error {
	archivePath := strings.Replace(filePath, directory, "", 1)
	archivePath = strings.TrimPrefix(archivePath, string(filepath.Separator))
	archivePath = filepath.ToSlash(archivePath)

	// Open the path to read from.
	f, err := os.Open(filePath)

	if err != nil {
		return err
	}

	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("Error closing file: %v\n", err)
		}
	}()

	info, err := f.Stat()

	if err != nil {
		return err
	}

	// Create the path in the tarball.
	header, err := tar.FileInfoHeader(info, archivePath)

	if err != nil {
		return err
	}

	header.Name = archivePath

	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	// Write the source file into the tarball at path from Create().
	if _, err = io.Copy(tarWriter, f); err != nil {
		return err
	}

	return nil
}

// ServeDirectory sends a directory via stdout and a simple wire protocol.
// The directory is added to a tar archive and compressed with gzip.
func ServeDirectory(srcDirectory string, dst io.WriteCloser) error {
	gzipWriter := gzip.NewWriter(dst)

	defer func() {
		if err := gzipWriter.Close(); err != nil {
			fmt.Printf("Error closing gzip writer: %v\n", err)
		}
	}()

	tarWriter := tar.NewWriter(gzipWriter)

	defer func() {
		if err := tarWriter.Close(); err != nil {
			fmt.Printf("Error closing tar writer: %v\n", err)
		}
	}()

	// TODO: Might be nice to stick file count or archive size at the top of our stream for determining progress.

	err := filepath.WalkDir(srcDirectory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the source directory root. This can be ignored because the source
		// directory root is just the root of the tarball.
		if path == srcDirectory {
			return nil
		}

		// Skip directories. Directories will be created automatically from paths to
		// each file to tar up.
		if d.IsDir() {
			return nil
		}

		return AddFileToTarArchive(tarWriter, path, srcDirectory)
	})

	if err != nil {
		return err
	}

	return nil
}

// Serve sends a file via stdout and a simple wire protocol.
func Serve(srcFilePath string, dst io.WriteCloser) error {
	fileBytes, err := os.ReadFile(srcFilePath)

	if err != nil {
		return err
	}

	// One line containing the file size in Bytes
	fileSize := len(fileBytes)
	_, err = fmt.Fprintf(dst, "%d\n", fileSize)

	if err != nil {
		return err
	}

	fileInfo, err := os.Stat(srcFilePath)

	if err != nil {
		return err
	}

	// One line containing the file mode
	// Normally this is represented in octal, but we're sending it over the wire in base-10 for
	// more straightforward [un]marshalling.
	fileMode := uint32(fileInfo.Mode())
	_, err = fmt.Fprintf(dst, "%d\n", fileMode)

	if err != nil {
		return err
	}

	// The file contents
	_, err = dst.Write(fileBytes)

	if err != nil {
		return err
	}

	return nil
}
