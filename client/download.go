package client

import (
	"archive/tar"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

func DownloadDirectory(client *ssh.Client, sourceDirectory, destDirectory string) error {
	session, err := client.NewSession()

	if err != nil {
		return err
	}

	defer func() {
		if err := session.Close(); err != nil && err != io.EOF {
			fmt.Printf("error when closing session: %v\n", err)
		}
	}()

	stdout, err := session.StdoutPipe()

	if err != nil {
		return err
	}

	stderr, err := session.StderrPipe()

	if err != nil {
		return err
	}

	// TODO: This shouldn't be hardcoded
	executable := "/home/ldonovan/bin/qcp"
	serveCmd := fmt.Sprintf("%s -d serve %s", executable, sourceDirectory)

	err = session.Start(serveCmd)

	if err != nil {
		return err
	}

	// TODO: Fix gzip problems

	// gzipReader, err := gzip.NewReader(stdout)
	//
	// if err != nil {
	// 	return err
	// }

	tarReader := tar.NewReader(stdout)

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
		filePath := path.Join(destDirectory, header.Name)
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

	go func(stderr io.Reader) {
		stderrReader := bufio.NewReader(stderr)

		for {
			out, err := stderrReader.ReadString('\n')

			if err == io.EOF {
				break
			}

			if err != nil {
				fmt.Printf("error when reading: %v\n", err)
				break
			}

			fmt.Printf("got stderr: %s\n", out)
		}
	}(stderr)

	err = session.Wait()

	if err != nil {
		return err
	}

	return nil
}

func Download(client *ssh.Client, sourceFilePath, destFilePath string) error {
	session, err := client.NewSession()

	if err != nil {
		return err
	}

	defer func() {
		if err := session.Close(); err != nil && err != io.EOF {
			fmt.Printf("error when closing session: %v\n", err)
		}
	}()

	stdout, err := session.StdoutPipe()

	if err != nil {
		return err
	}

	stderr, err := session.StderrPipe()

	if err != nil {
		return err
	}

	// TODO: This shouldn't be hardcoded
	executable := "/home/ldonovan/bin/qcp"
	serveCmd := fmt.Sprintf("%s serve %s", executable, sourceFilePath)

	err = session.Start(serveCmd)

	if err != nil {
		return err
	}

	stdoutReader := bufio.NewReader(stdout)

	fileSizeStr, err := stdoutReader.ReadString('\n')

	if err != nil {
		return err
	}

	fileSize, err := strconv.Atoi(strings.TrimSpace(fileSizeStr))

	if err != nil {
		return err
	}

	fileModeStr, err := stdoutReader.ReadString('\n')

	if err != nil {
		return err
	}

	fileMode, err := strconv.Atoi(strings.TrimSpace(fileModeStr))

	if err != nil {
		return err
	}

	fileContents := make([]byte, fileSize)
	_, err = io.ReadFull(stdoutReader, fileContents)

	if err != nil {
		return err
	}

	err = os.WriteFile(destFilePath, fileContents, os.FileMode(fileMode))

	if err != nil {
		return err
	}

	go func(stderr io.Reader) {
		stderrReader := bufio.NewReader(stderr)

		for {
			out, err := stderrReader.ReadString('\n')

			if err == io.EOF {
				break
			}

			if err != nil {
				fmt.Printf("error when reading: %v\n", err)
				break
			}

			fmt.Printf("got stderr: %s\n", out)
		}
	}(stderr)

	err = session.Wait()

	if err != nil {
		return err
	}

	return nil
}
