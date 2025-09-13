package sessions

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

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

func StartDownload(client *ssh.Client, filepaths []string, compress bool, offset int64) (DownloadSession, error) {
	executable, err := common.FindExecutable(client, "qcp")

	if err != nil {
		return nil, fmt.Errorf("find executable: %w", err)
	}

	cmd, err := protocol.Parser.Marshal(executable, map[string]any{
		"mode":         "serve",
		"sources":      filepaths,
		"uncompressed": !compress,
		"offset":       fmt.Sprintf("%d", offset), // TODO: This is a goparse limitation. It calls Sprintf with %s internally, when it should use %v.
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

	isDir := f[0]&protocol.IsDirectory > 0
	isCompressed := f[0]&protocol.IsCompressed > 0

	downloadInfo := serve.DownloadInfo{
		Filename:   filename,
		Contents:   src,
		Directory:  isDir,
		Compressed: isCompressed,
	}

	if !isDir {
		fileSizeBytes := make([]byte, 4)
		fileModeBytes := make([]byte, 4)

		if _, err := src.Read(fileSizeBytes); err != nil {
			return serve.DownloadInfo{}, fmt.Errorf("read file size: %w", err)
		}

		if _, err := src.Read(fileModeBytes); err != nil {
			return serve.DownloadInfo{}, fmt.Errorf("read file mode: %w", err)
		}

		downloadInfo.Size = binary.LittleEndian.Uint32(fileSizeBytes)
		downloadInfo.Mode = os.FileMode(binary.LittleEndian.Uint32(fileModeBytes))
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

func Download(client *ssh.Client, srcFilePaths []string, dstFilePath string, compress bool) error {
	// When we're downloading a single file or directory and dstFilePath isn't provided,
	// we need to set the destination filename to the basename of the source filepath.

	if dstFilePath == "" && len(srcFilePaths) == 1 {
		dstFilePath = filepath.Base(srcFilePaths[0])
	}

	// We are downloading a single file, so this could be a partial download.
	// TODO: Support partial downloads of directories.

	var offset int64 = 0

	if len(srcFilePaths) == 1 {
		partialFilename := dstFilePath + ".partial"

		info, err := os.Stat(partialFilename)

		if err == nil {
			fmt.Printf("Resuming partial download of %s\n", partialFilename)
			offset = info.Size()
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat %s: %w", partialFilename, err)
		}
	}

	session, err := StartDownload(client, srcFilePaths, compress, offset)

	if err != nil {
		return fmt.Errorf("start download: %w", err)
	}

	defer session.Stop()

	downloadInfo, err := session.GetDownloadInfo(dstFilePath)

	if err != nil {
		return fmt.Errorf("get download info: %w", err)
	}

	if err := downloadInfo.Receive(); err != nil {
		return fmt.Errorf("receive %v: %w", srcFilePaths, err)
	}

	return nil
}
