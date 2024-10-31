package sideload

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/l-donovan/qcp/common"
	"golang.org/x/crypto/ssh"
)

func getBinaryUrl(release, hostOs, hostArch string) (string, error) {
	var url string
	client := http.Client{}

	if release == "latest" {
		url = "https://api.github.com/repos/l-donovan/qcp/releases/latest"
	} else {
		url = fmt.Sprintf("https://api.github.com/repos/l-donovan/qcp/releases/tags/%s", release)
	}

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("Accept", "application/vnd.github+json")
	req.Header.Add("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)

	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("failed to get release %s, request returned %s", release, resp.Status)
	}

	res, err := io.ReadAll(resp.Body)

	if err != nil {
		return "", err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error when closing repsonse body: %v\n", err)
		}
	}()

	var releaseInfo map[string]any
	err = json.Unmarshal(res, &releaseInfo)

	if err != nil {
		return "", err
	}

	for _, rawAsset := range releaseInfo["assets"].([]any) {
		asset := rawAsset.(map[string]any)
		name := asset["name"].(string)
		downloadUrl := asset["browser_download_url"].(string)

		if strings.HasSuffix(name, fmt.Sprintf("%s-%s.tar.gz", hostOs, hostArch)) {
			return downloadUrl, nil
		}
	}

	return "", fmt.Errorf("could not find asset")
}

func transferBinary(client *ssh.Client, tarReader *tar.Reader, location string) error {
	err := common.RunWithPipes(client, fmt.Sprintf("cat > %s", location), func(stdin io.WriteCloser, stdout, stderr io.Reader) error {
		_, err := io.Copy(stdin, tarReader)
		return err
	})

	if err != nil {
		return err
	}

	err = makeExecutable(client, location)

	if err != nil {
		return err
	}

	return nil
}

func makeExecutable(client *ssh.Client, location string) error {
	session, err := client.NewSession()

	if err != nil {
		return err
	}

	defer func() {
		if err := session.Close(); err != nil && err != io.EOF {
			_, _ = fmt.Fprintf(os.Stderr, "error when closing session: %v\n", err)
		}
	}()

	_, err = session.Output(fmt.Sprintf("chmod +x %s", location))

	return err
}

func GetBinary(client *ssh.Client, release, location string) error {
	hostOs, err := getOs(client)

	if err != nil {
		panic(err)
	}

	hostArch, err := getArch(client)

	if err != nil {
		panic(err)
	}

	url, err := getBinaryUrl(release, hostOs, hostArch)

	if err != nil {
		return err
	}

	resp, err := http.Get(url)

	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to download release from %s, request returned %s", url, resp.Status)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error when closing body: %v\n", err)
		}
	}()

	gzipReader, err := gzip.NewReader(resp.Body)

	if err != nil {
		return err
	}

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		if header.Name == "qcp" {
			return transferBinary(client, tarReader, location)
		}

		_, _ = io.Copy(io.Discard, tarReader)
	}

	return fmt.Errorf("could not find file matching qcp in downloaded tarball")
}
