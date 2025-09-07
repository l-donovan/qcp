package serve

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/l-donovan/qcp/common"
)

const (
	printFrequency = 8
)

type DownloadInfo struct {
	Filename   string
	Contents   io.Reader
	Directory  bool
	Compressed bool
	Mode       os.FileMode
	Size       uint32
	Progress   chan int64
}

func (d DownloadInfo) receiveDirectory() error {
	gzipReader, err := gzip.NewReader(d.Contents)

	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()

		if err != nil {
			if err == io.EOF {
				fmt.Print("All files received\n")
				return nil
			}

			return fmt.Errorf("read tar: %w", err)
		}

		filePath := path.Join(d.Filename, header.Name)
		fileInfo := header.FileInfo()

		if fileInfo.IsDir() {
			fmt.Printf("Creating directory %s\n", filePath)

			if err := os.MkdirAll(filePath, 0o777); err != nil {
				return fmt.Errorf("create directory %s: %w", filePath, err)
			}
		} else {
			fmt.Printf("Receiving %s\n", filePath)

			if err := os.MkdirAll(filepath.Dir(filePath), 0o775); err != nil {
				return fmt.Errorf("create directory %s: %w", filepath.Dir(filePath), err)
			}

			fp, err := os.Create(filePath)

			if err != nil {
				return fmt.Errorf("create %s: %w", filePath, err)
			}

			if err := fp.Truncate(fileInfo.Size()); err != nil {
				return fmt.Errorf("set file size: %w", err)
			}

			if _, err := io.Copy(fp, tarReader); err != nil {
				return fmt.Errorf("write %s: %w", filePath, err)
			}

			if err := fp.Chmod(fileInfo.Mode()); err != nil {
				return fmt.Errorf("change filemode: %w", err)
			}

			if err := fp.Close(); err != nil {
				return fmt.Errorf("close file %s: %w", filePath, err)
			}
		}
	}
}

func (d DownloadInfo) receiveFileCompressed() error {
	gzipReader, err := gzip.NewReader(d.Contents)

	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}

	fp, err := os.Create(d.Filename)

	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	defer func() {
		if err := fp.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error closing file: %v\n", err)
		}
	}()

	if _, err := io.Copy(fp, gzipReader); err != nil {
		return fmt.Errorf("copy to destination file: %w", err)
	}

	if err := fp.Chmod(d.Mode); err != nil {
		return fmt.Errorf("set destination file permissions: %w", err)
	}

	return nil
}

func (d DownloadInfo) receiveFileUncompressed() error {
	fp, err := os.Create(d.Filename)

	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	defer func() {
		if err := fp.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error closing file: %v\n", err)
		}
	}()

	if err := fp.Truncate(int64(d.Size)); err != nil {
		return fmt.Errorf("set file size: %w", err)
	}

	if _, err := io.Copy(fp, d.Contents); err != nil {
		return fmt.Errorf("copy to destination file: %w", err)
	}

	if err := fp.Chmod(d.Mode); err != nil {
		return fmt.Errorf("set destination file permissions: %w", err)
	}

	return nil
}

func (d DownloadInfo) Receive() error {
	// There's no special handling required for receiving multiple files, as they'll always
	// arrive as a compressed directory.
	if d.Directory {
		return d.receiveDirectory()
	} else if d.Compressed {
		return d.receiveFileCompressed()
	} else {
		return d.receiveFileUncompressed()
	}
}

func (d DownloadInfo) ReceiveWeb(w http.ResponseWriter) {
	flusher, ok := w.(http.Flusher)

	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "Could not treat http.ResponseWriter as http.Flusher")
		return
	}

	var mimeType string

	if d.Directory {
		mimeType = "application/gzip"
	} else {
		mimeType = mime.TypeByExtension(path.Ext(d.Filename))

		if mimeType == "" {
			mimeType = "text/plain"
		}
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if d.Size != 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", d.Size))
	}

	if d.Directory {
		w.Header().Set("Content-Disposition", "attachment;filename="+d.Filename+".tar.gz")
	} else {
		w.Header().Set("Content-Disposition", "attachment;filename="+d.Filename)
	}

	// We don't set Content-Encoding: gzip for directories even though they are
	// sent as tar.gz files, because we don't want the browser to prematurely decompress
	// the archive.

	if !d.Directory && d.Compressed {
		w.Header().Set("Content-Encoding", "gzip")
	}

	defer func() {
		if d.Progress != nil {
			d.Progress <- -1
		}
	}()

	var totalCopied int64

	for {
		n, err := io.CopyN(w, d.Contents, 1024)

		if err != nil {
			if err != io.EOF {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = fmt.Fprintf(w, "download: %v", err)
			}

			return
		}

		totalCopied += n

		if d.Progress != nil {
			d.Progress <- totalCopied
		}

		flusher.Flush()
	}
}

func (d *DownloadInfo) PrintProgressBar() {
	d.Progress = make(chan int64)

	printPeriod := time.Duration(1000/printFrequency) * time.Millisecond

	lastTime := time.Now()
	lastProgressBytes := int64(0)
	lastLen := 0

	for progressBytes := range d.Progress {
		timeDelta := time.Since(lastTime)

		// We effectively need to lower the resolution so that we have a large
		// enough time delta from which we can compute download speed.

		if timeDelta < printPeriod {
			continue
		}

		speedBitsPerSecond := 1_000 * 8 * (progressBytes - lastProgressBytes) / timeDelta.Milliseconds()

		if lastLen == 0 {
			fmt.Print("\033[?25lTransferred ")
		} else {
			fmt.Printf("\033[%dD", lastLen)
		}

		if progressBytes == -1 {
			break
		}

		prettySize := common.PrettifySize(progressBytes)
		prettySpeed := common.PrettifySpeed(speedBitsPerSecond)

		text := fmt.Sprintf("%s (%s)", prettySize, prettySpeed)
		fmt.Printf("\033[K%s", text)

		lastTime = time.Now()
		lastProgressBytes = progressBytes
		lastLen = len(text)
	}

	fmt.Println("\033[?25h")
}
