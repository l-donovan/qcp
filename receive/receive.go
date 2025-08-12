package receive

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

type LogFunc func(fmt string, a ...any) (n int, err error)

func ReceiveDirectory(dstDirectory string, src io.Reader, log LogFunc) error {
	gzipReader, err := gzip.NewReader(src)

	if err != nil {
		return err
	}

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			log("All files received")
			break
		}

		if err != nil {
			return err
		}

		var fileBuf bytes.Buffer
		filePath := path.Join(dstDirectory, header.Name)
		log("Receiving %s\n", header.Name)

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
	srcReader := bufio.NewReader(src)

	fileModeStr, err := srcReader.ReadString('\n')

	if err != nil {
		return err
	}

	fileMode, err := strconv.Atoi(strings.TrimSpace(fileModeStr))

	if err != nil {
		return err
	}

	gzipReader, err := gzip.NewReader(src)

	if err != nil {
		return err
	}

	fp, err := os.Create(dstFilePath)

	if err != nil {
		return fmt.Errorf("create file: %v", err)
	}

	if _, err := io.Copy(fp, gzipReader); err != nil {
		return fmt.Errorf("copy to destination file: %v", err)
	}

	if err := fp.Chmod(os.FileMode(fileMode)); err != nil {
		return fmt.Errorf("set destination file permissions: %v", err)
	}

	return nil
}

func receiveFileUncompressed(dstFilePath string, src io.Reader) error {
	srcReader := bufio.NewReader(src)

	fileSizeStr, err := srcReader.ReadString('\n')

	if err != nil {
		return err
	}

	fileSize, err := strconv.Atoi(strings.TrimSpace(fileSizeStr))

	if err != nil {
		return err
	}

	fileModeStr, err := srcReader.ReadString('\n')

	if err != nil {
		return err
	}

	fileMode, err := strconv.Atoi(strings.TrimSpace(fileModeStr))

	if err != nil {
		return err
	}

	fileContents := make([]byte, fileSize)

	if _, err := io.ReadFull(srcReader, fileContents); err != nil {
		return err
	}

	if err := os.WriteFile(dstFilePath, fileContents, os.FileMode(fileMode)); err != nil {
		return err
	}

	return nil
}

func ReceiveFile(dstFilePath string, src io.Reader, compressed bool) error {
	if compressed {
		return receiveFileCompressed(dstFilePath, src)
	} else {
		return receiveFileUncompressed(dstFilePath, src)
	}
}
