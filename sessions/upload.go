package sessions

import (
	"fmt"

	"github.com/l-donovan/qcp/common"
	"github.com/l-donovan/qcp/protocol"
	"github.com/l-donovan/qcp/serve"
	"golang.org/x/crypto/ssh"
)

type UploadSession interface {
	GetUploadInfo(filename string) serve.UploadInfo
	Wait()
}

type uploadSession common.Session

func StartUpload(client *ssh.Client, filepath string) (UploadSession, error) {
	executable, err := common.FindExecutable(client, "qcp")

	if err != nil {
		return nil, fmt.Errorf("find executable: %w", err)
	}

	cmd, err := protocol.Parser.Marshal(executable, map[string]any{
		"mode":        "receive",
		"destination": filepath,
	})

	if err != nil {
		return nil, fmt.Errorf("generate command: %w", err)
	}

	session, err := common.Start(client, cmd)

	if err != nil {
		return nil, fmt.Errorf("start session: %w", err)
	}

	return uploadSession(session), nil
}

func (s uploadSession) GetUploadInfo(filename string) serve.UploadInfo {
	return serve.UploadInfo{
		Filenames:   []string{filename},
		Destination: s.Stdin,
	}
}

func (s uploadSession) Wait() {
	s.Stdin.Close()
	s.Session.Wait()
}

func Upload(client *ssh.Client, srcFilePath, dstFilePath string) error {
	session, err := StartUpload(client, dstFilePath)

	if err != nil {
		return fmt.Errorf("receive %s: %w", dstFilePath, err)
	}

	defer session.Wait()

	uploadInfo := session.GetUploadInfo(srcFilePath)

	if err := uploadInfo.Serve(); err != nil {
		return fmt.Errorf("serve %s: %w", srcFilePath, err)
	}

	return nil
}
