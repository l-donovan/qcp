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
	Filename     string
	Contents     io.Reader
	ShouldUnpack bool
	Mode         os.FileMode
	Progress     chan int64
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

		fp, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o666)

		if err != nil {
			return fmt.Errorf("create %s: %w", filePath, err)
		}

		var dest io.Writer = fp

		defer func() {
			// Close returns an error if the file pointer has already been closed.
			// Under normal operation we expect the file to be closed before we
			// get here, so we ignore the potential error.

			_ = fp.Close()
		}()

		localFileInfo, err := fp.Stat()

		if err != nil {
			return fmt.Errorf("stat %s: %w", filePath, err)
		}

		if localFileInfo.Size() == fileInfo.Size() {
			// Skip.

			dest = io.Discard
		} else if localFileInfo.Size() > fileInfo.Size() {
			// Truncate.

			if _, err := fp.Seek(0, io.SeekStart); err != nil {
				return fmt.Errorf("rewind %s: %w", filePath, err)
			}

			if err := os.Truncate(filePath, 0); err != nil {
				return fmt.Errorf("truncate %s: %w", filePath, err)
			}
		}

		if err := os.Truncate(filePath, fileInfo.Size()); err != nil {
			return fmt.Errorf("set file size: %w", err)
		}

		if _, err := io.Copy(dest, src); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}

		if err := fp.Chmod(fileInfo.Mode()); err != nil {
			return fmt.Errorf("change filemode: %w", err)
		}
	}

	return nil
}

func (d DownloadInfo) Receive(progressFile *os.File) error {
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

		if progressFile != nil {
			if _, err := progressFile.Seek(0, io.SeekStart); err != nil {
				return fmt.Errorf("rewind progress file: %w", err)
			}

			if err := progressFile.Truncate(0); err != nil {
				return fmt.Errorf("truncate progress file: %w", err)
			}

			if _, err := progressFile.WriteString(filePath); err != nil {
				return fmt.Errorf("write progress file: %w", err)
			}
		}

		if err := receiveTarEntry(fileInfo, filePath, tarReader); err != nil {
			return fmt.Errorf("receive tar entry: %w", err)
		}
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

	if d.ShouldUnpack {
		mimeType = mime.TypeByExtension(path.Ext(d.Filename))

		if mimeType == "" {
			mimeType = "text/plain"
		}
	} else {
		mimeType = "application/gzip"
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	defer func() {
		if d.Progress != nil {
			d.Progress <- -1
		}
	}()

	var dst io.Writer
	var src io.Reader

	if d.ShouldUnpack {
		gzipReader, err := gzip.NewReader(d.Contents)

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, "Could not create gzip reader: %v", err)
			return
		}

		tarReader := tar.NewReader(gzipReader)

		_, err = tarReader.Next()

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, "Could not create tar reader: %v", err)
			return
		}

		gzipWriter := gzip.NewWriter(w)

		defer func() {
			if err := gzipWriter.Close(); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Failed to close gzip writer: %v\n", err)
			}
		}()

		w.Header().Set("Content-Disposition", "attachment;filename="+d.Filename)
		w.Header().Set("Content-Encoding", "gzip")

		dst = gzipWriter
		src = tarReader
	} else {
		w.Header().Set("Content-Disposition", "attachment;filename="+d.Filename+".tar.gz")
		// We don't set Content-Encoding: gzip for directories even though they are
		// sent as tar.gz files, because we don't want the browser to prematurely decompress
		// the archive.

		dst = w
		src = d.Contents
	}

	var totalCopied int64

	for {
		n, err := io.CopyN(dst, src, 1024)

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
