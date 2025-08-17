package receive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/l-donovan/qcp/protocol"
)

func receiveDirectory(dstDirectory string, src io.Reader) error {
	gzipReader, err := gzip.NewReader(src)

	if err != nil {
		return err
	}

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			fmt.Printf("All files received")
			break
		}

		if err != nil {
			return err
		}

		var fileBuf bytes.Buffer
		filePath := path.Join(dstDirectory, header.Name)
		fmt.Printf("Receiving %s\n", header.Name)

		if _, err := io.Copy(&fileBuf, tarReader); err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Dir(filePath), 0o777); err != nil {
			return err
		}

		if err := os.WriteFile(filePath, fileBuf.Bytes(), header.FileInfo().Mode()); err != nil {
			return err
		}
	}

	return nil
}

func receiveFileCompressed(dstFilePath string, src io.Reader) error {
	fileModeBytes := make([]byte, 4)

	if _, err := src.Read(fileModeBytes); err != nil {
		return fmt.Errorf("read file mode: %v", err)
	}

	fileMode := binary.LittleEndian.Uint32(fileModeBytes)

	gzipReader, err := gzip.NewReader(src)

	if err != nil {
		return fmt.Errorf("create gzip reader: %v", err)
	}

	fp, err := os.Create(dstFilePath)

	if err != nil {
		return fmt.Errorf("create file: %v", err)
	}

	defer func() {
		if err := fp.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing file: %v\n", err)
		}
	}()

	if _, err := io.Copy(fp, gzipReader); err != nil {
		return fmt.Errorf("copy to destination file: %v", err)
	}

	if err := fp.Chmod(os.FileMode(fileMode)); err != nil {
		return fmt.Errorf("set destination file permissions: %v", err)
	}

	return nil
}

func receiveFileUncompressed(dstFilePath string, src io.Reader) error {
	fileSizeBytes := make([]byte, 4)
	fileModeBytes := make([]byte, 4)

	if _, err := src.Read(fileSizeBytes); err != nil {
		return fmt.Errorf("read file size: %v", err)
	}

	if _, err := src.Read(fileModeBytes); err != nil {
		return fmt.Errorf("read file mode: %v", err)
	}

	fileSize := binary.LittleEndian.Uint32(fileSizeBytes)
	fileMode := binary.LittleEndian.Uint32(fileModeBytes)

	fp, err := os.Create(dstFilePath)

	if err != nil {
		return fmt.Errorf("create file: %v", err)
	}

	defer func() {
		if err := fp.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing file: %v\n", err)
		}
	}()

	if err := fp.Truncate(int64(fileSize)); err != nil {
		return fmt.Errorf("set file size: %v", err)
	}

	if _, err := io.Copy(fp, src); err != nil {
		return fmt.Errorf("copy to destination file: %v", err)
	}

	if err := fp.Chmod(os.FileMode(fileMode)); err != nil {
		return fmt.Errorf("set destination file permissions: %v", err)
	}

	return nil
}

func Receive(dstFilePath string, src io.Reader) error {
	f := make([]byte, 1)

	if _, err := src.Read(f); err != nil {
		return fmt.Errorf("read flags: %v", err)
	}

	isDir := f[0]&protocol.IsDirectory > 0
	isCompressed := f[0]&protocol.IsCompressed > 0

	if isDir {
		return receiveDirectory(dstFilePath, src)
	} else if isCompressed {
		return receiveFileCompressed(dstFilePath, src)
	} else {
		return receiveFileUncompressed(dstFilePath, src)
	}
}
