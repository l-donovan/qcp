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

func ReceiveDirectory(dstDirectory string, src io.Reader) error {
	gzipReader, err := gzip.NewReader(src)

	if err != nil {
		return err
	}

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			fmt.Println("All files received")
			break
		}

		if err != nil {
			return err
		}

		var fileBuf bytes.Buffer
		filePath := path.Join(dstDirectory, header.Name)
		fmt.Printf("Receiving %s\n", header.Name)

		_, err = io.Copy(&fileBuf, tarReader)

		if err != nil {
			return err
		}

		err = os.MkdirAll(filepath.Dir(filePath), 0o777)

		if err != nil {
			return err
		}

		err = os.WriteFile(filePath, fileBuf.Bytes(), header.FileInfo().Mode())

		if err != nil {
			return err
		}
	}

	return nil
}

func Receive(dstFilePath string, src io.Reader) error {
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
	_, err = io.ReadFull(srcReader, fileContents)

	if err != nil {
		return err
	}

	err = os.WriteFile(dstFilePath, fileContents, os.FileMode(fileMode))

	if err != nil {
		return err
	}

	return nil
}
