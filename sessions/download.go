package sessions

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/l-donovan/qcp/common"
	"github.com/l-donovan/qcp/protocol"
	"github.com/l-donovan/qcp/serve"
	"golang.org/x/crypto/ssh"

	"al.essio.dev/pkg/shellescape"
)

type DownloadSession interface {
	GetDownloadInfo(filename string) (serve.DownloadInfo, error)
	Stop()
}

type downloadSession common.Session

func StartDownload(client *ssh.Client, filepaths []string, compress bool) (DownloadSession, error) {
	executable, err := common.FindExecutable(client, "qcp")

	if err != nil {
		return nil, fmt.Errorf("find executable: %v", err)
	}

	for i, filePath := range filepaths {
		filepaths[i] = shellescape.Quote(filePath)
	}

	cmd := fmt.Sprintf("%s serve %s", executable, strings.Join(filepaths, " "))

	if !compress {
		cmd += " -u"
	}

	session, err := common.Start(client, cmd)

	if err != nil {
		return nil, fmt.Errorf("start session: %v", err)
	}

	return downloadSession(session), nil
}

func GetDownloadInfo(filename string, src io.Reader) (serve.DownloadInfo, error) {
	f := make([]byte, 1)

	if _, err := src.Read(f); err != nil {
		return serve.DownloadInfo{}, fmt.Errorf("read flags: %v", err)
	}

	isDir := f[0]&protocol.IsDirectory > 0
	isCompressed := f[0]&protocol.IsCompressed > 0

	downloadInfo := serve.DownloadInfo{
		Filename:   filename,
		Contents:   src,
		Directory:  isDir,
		Compressed: isCompressed,
	}

	if isDir {
		// Requires no special treatment.
	} else if isCompressed {
		fileModeBytes := make([]byte, 4)

		if _, err := src.Read(fileModeBytes); err != nil {
			return serve.DownloadInfo{}, fmt.Errorf("read file mode: %v", err)
		}

		downloadInfo.Mode = os.FileMode(binary.LittleEndian.Uint32(fileModeBytes))
	} else {
		fileSizeBytes := make([]byte, 4)
		fileModeBytes := make([]byte, 4)

		if _, err := src.Read(fileSizeBytes); err != nil {
			return serve.DownloadInfo{}, fmt.Errorf("read file size: %v", err)
		}

		if _, err := src.Read(fileModeBytes); err != nil {
			return serve.DownloadInfo{}, fmt.Errorf("read file mode: %v", err)
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
	session, err := StartDownload(client, srcFilePaths, compress)

	if err != nil {
		return fmt.Errorf("start download: %v", err)
	}

	defer session.Stop()

	downloadInfo, err := session.GetDownloadInfo(dstFilePath)

	if err != nil {
		return fmt.Errorf("get download info: %v", err)
	}

	if err := downloadInfo.Receive(); err != nil {
		return fmt.Errorf("receive %v: %v", srcFilePaths, err)
	}

	return nil
}
