package serve

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
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

func receiveTarEntry(fileInfo fs.FileInfo, filePath string, src *tar.Reader) error {
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

		defer func() {
			// Close returns an error if the file pointer has already been closed.
			// Under normal operation we expect the file to be closed before we
			// get here, so we ignore the potential error.

			_ = fp.Close()
		}()

		if err := fp.Truncate(fileInfo.Size()); err != nil {
			return fmt.Errorf("set file size: %w", err)
		}

		if err := fp.Chmod(fileInfo.Mode()); err != nil {
			return fmt.Errorf("change filemode: %w", err)
		}

		if _, err := io.Copy(fp, src); err != nil {
			return fmt.Errorf("write %s: %w", filePath, err)
		}

		// Unlike in the deferred function above, we do NOT expect Close to
		// return an error at this point.

		if err := fp.Close(); err != nil {
			return fmt.Errorf("close file %s: %w", filePath, err)
		}
	}

	return nil
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
				fmt.Println("All files received")
				return nil
			}

			return fmt.Errorf("read tar: %w", err)
		}

		filePath := path.Join(d.Filename, header.Name)
		fileInfo := header.FileInfo()

		if err := receiveTarEntry(fileInfo, filePath, tarReader); err != nil {
			return fmt.Errorf("receive tar entry: %v", err)
		}
	}
}

func (d DownloadInfo) receiveFile() error {
	var src io.Reader = d.Contents

	if d.Compressed {
		gzipReader, err := gzip.NewReader(d.Contents)

		if err != nil {
			return fmt.Errorf("create gzip reader: %w", err)
		}

		src = gzipReader
	}

	partialFilename := d.Filename + ".partial"

	fp, err := os.OpenFile(partialFilename, os.O_APPEND|os.O_CREATE, 0o666)

	if err != nil {
		return fmt.Errorf("open partial file: %w", err)
	}

	defer func() {
		_ = fp.Close()
	}()

	// TODO: Truncate is (hopefully temporarily) disabled.
	// I was running into some strange access denied issues.

	if err := fp.Chmod(d.Mode); err != nil {
		return fmt.Errorf("set destination file permissions: %w", err)
	}

	if _, err := io.Copy(fp, src); err != nil {
		return fmt.Errorf("copy to destination file: %w", err)
	}

	if err := fp.Close(); err != nil {
		return fmt.Errorf("close partial file: %w", err)
	}

	if err := os.Rename(partialFilename, d.Filename); err != nil {
		return fmt.Errorf("rename partial file: %w", err)
	}

	return nil
}

func (d DownloadInfo) Receive() error {
	// There's no special handling required for receiving multiple files, as they'll always
	// arrive as a compressed directory.

	if d.Directory {
		return d.receiveDirectory()
	} else {
		return d.receiveFile()
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
