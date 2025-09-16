package sessions

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/l-donovan/qcp/common"
	"github.com/l-donovan/qcp/protocol"
	"github.com/l-donovan/qcp/serve"
	"golang.org/x/crypto/ssh"
)

type DownloadSession interface {
	GetDownloadInfo(filename string) (serve.DownloadInfo, error)
	Stop()
}

type downloadSession common.Session

func StartDownload(client *ssh.Client, filepaths []string, offsetFile string, offsetPos int64) (DownloadSession, error) {
	executable, err := common.FindExecutable(client, "qcp")

	if err != nil {
		return nil, fmt.Errorf("find executable: %w", err)
	}

	cmd, err := protocol.Parser.Marshal(executable, map[string]any{
		"mode":        "serve",
		"sources":     filepaths,
		"offset-file": offsetFile,
		"offset-pos":  fmt.Sprintf("%d", offsetPos), // TODO: This is a goparse limitation. It calls Sprintf with %s internally, when it should use %v.
	})

	if err != nil {
		return nil, fmt.Errorf("generate command: %w", err)
	}

	session, err := common.Start(client, cmd)

	if err != nil {
		return nil, fmt.Errorf("start session: %w", err)
	}

	return downloadSession(session), nil
}

func GetDownloadInfo(filename string, src io.Reader) (serve.DownloadInfo, error) {
	f := make([]byte, 1)

	if _, err := src.Read(f); err != nil {
		return serve.DownloadInfo{}, fmt.Errorf("read flags: %w", err)
	}

	shouldUnpack := f[0]&protocol.ShouldUnpack > 0

	downloadInfo := serve.DownloadInfo{
		Filename:     filename,
		Contents:     src,
		ShouldUnpack: shouldUnpack,
	}

	return downloadInfo, nil
}

func (s downloadSession) GetDownloadInfo(filename string) (serve.DownloadInfo, error) {
	return GetDownloadInfo(filename, s.Stdout)
}

func (s downloadSession) Stop() {
	s.Session.Signal(ssh.SIGQUIT)
	s.Session.Close()
}

func Download(client *ssh.Client, srcFilePaths []string, dstFilePath string) error {
	var offsetFile string
	var offsetPos int64

	// Check for a partially downloaded directory.
	progressFilename := common.CreateIdentifier(srcFilePaths) + ".progress"

	progressFile, err := os.OpenFile(progressFilename, os.O_RDWR|os.O_CREATE, 0o644)

	if err != nil {
		return fmt.Errorf("open %s: %w", progressFilename, err)
	}

	defer func() {
		_ = progressFile.Close()
	}()

	contents, err := io.ReadAll(progressFile)

	if err == nil {
		if len(contents) > 0 {
			offsetFile = strings.TrimSpace(string(contents))

			fileInfo, err := os.Stat(offsetFile)

			if err != nil {
				return fmt.Errorf("get current file info: %w", err)
			}

			offsetPos = fileInfo.Size()

			fmt.Printf("Resuming download at %s (already downloaded %s)\n", offsetFile, common.PrettifySize(offsetPos))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", progressFilename, err)
	}

	session, err := StartDownload(client, srcFilePaths, offsetFile, offsetPos)

	if err != nil {
		return fmt.Errorf("start download: %w", err)
	}

	defer session.Stop()

	downloadInfo, err := session.GetDownloadInfo(dstFilePath)

	if err != nil {
		return fmt.Errorf("get download info: %w", err)
	}

	if err := downloadInfo.Receive(progressFile); err != nil {
		return fmt.Errorf("receive %v: %w", srcFilePaths, err)
	}

	if err := progressFile.Close(); err != nil {
		return fmt.Errorf("close progress file: %w", err)
	}

	if err := os.Remove(progressFilename); err != nil {
		return fmt.Errorf("remove progress file: %w", err)
	}

	return nil
}
