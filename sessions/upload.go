package sessions

import (
	"fmt"
	"github.com/l-donovan/qcp/common"
	"github.com/l-donovan/qcp/serve"
	"golang.org/x/crypto/ssh"
	"io"
	"os"
)

type UploadSession interface {
	GetUploadInfo(filename string, compress bool) (serve.UploadInfo, error) // TODO: This needs a better name. It doesn't really download.
	Wait()
}
type uploadSession common.Session

func StartUpload(client *ssh.Client, filepath string) (UploadSession, error) {
	executable, err := common.FindExecutable(client, "qcp")

	if err != nil {
		return nil, fmt.Errorf("find executable: %v", err)
	}

	cmd := fmt.Sprintf("%s receive %s", executable, filepath)

	session, err := common.Start(client, cmd)

	if err != nil {
		return nil, fmt.Errorf("start session: %v", err)
	}

	return uploadSession(session), nil
}

func GetUploadInfo(filename string, compress bool, dst io.WriteCloser) (serve.UploadInfo, error) {
	fileInfo, err := os.Stat(filename)

	if err != nil {
		return serve.UploadInfo{}, err
	}

	if fileInfo.IsDir() {
		return serve.UploadInfo{
			Filenames:   []string{filename},
			Destination: dst,
			Directory:   true,
			Compressed:  true,
		}, nil
	} else {
		return serve.UploadInfo{
			Filenames:   []string{filename},
			Destination: dst,
			Directory:   false,
			Compressed:  compress,
		}, nil
	}
}

func (s uploadSession) GetUploadInfo(filename string, compress bool) (serve.UploadInfo, error) {
	return GetUploadInfo(filename, compress, s.Stdin)
}

func (s uploadSession) Wait() {
	s.Stdin.Close()
	s.Session.Wait()
}

func Upload(client *ssh.Client, srcFilePath, dstFilePath string, compress bool) error {
	session, err := StartUpload(client, dstFilePath)

	if err != nil {
		return fmt.Errorf("receive %s: %v", dstFilePath, err)
	}

	defer session.Wait()

	uploadInfo, err := session.GetUploadInfo(srcFilePath, compress)

	if err != nil {
		return fmt.Errorf("get upload info: %v", err)
	}

	if err := uploadInfo.Serve(); err != nil {
		return fmt.Errorf("serve %s: %v", srcFilePath, err)
	}

	return nil
}
