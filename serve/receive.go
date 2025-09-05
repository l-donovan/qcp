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
		return fmt.Errorf("create gzip reader: %v", err)
	}

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()

		if err != nil {
			if err == io.EOF {
				fmt.Print("All files received\n")
				return nil
			}

			return fmt.Errorf("read tar: %v", err)
		}

		filePath := path.Join(d.Filename, header.Name)
		fileInfo := header.FileInfo()

		if fileInfo.IsDir() {
			fmt.Printf("Creating directory %s\n", filePath)

			if err := os.MkdirAll(filePath, 0o777); err != nil {
				return fmt.Errorf("create directory %s: %v", filePath, err)
			}
		} else {
			fmt.Printf("Receiving %s\n", filePath)

			if err := os.MkdirAll(filepath.Dir(filePath), 0o775); err != nil {
				return fmt.Errorf("create directory %s: %v", filepath.Dir(filePath), err)
			}

			fp, err := os.Create(filePath)

			if err != nil {
				return fmt.Errorf("create %s: %v", filePath, err)
			}

			if err := fp.Truncate(fileInfo.Size()); err != nil {
				return fmt.Errorf("set file size: %v", err)
			}

			if _, err := io.Copy(fp, tarReader); err != nil {
				return fmt.Errorf("write %s: %v", filePath, err)
			}

			if err := fp.Chmod(fileInfo.Mode()); err != nil {
				return fmt.Errorf("change filemode: %v", err)
			}

			if err := fp.Close(); err != nil {
				return fmt.Errorf("close file %s: %v", filePath, err)
			}
		}
	}
}

func (d DownloadInfo) receiveFileCompressed() error {
	gzipReader, err := gzip.NewReader(d.Contents)

	if err != nil {
		return fmt.Errorf("create gzip reader: %v", err)
	}

	fp, err := os.Create(d.Filename)

	if err != nil {
		return fmt.Errorf("create file: %v", err)
	}

	defer func() {
		if err := fp.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error closing file: %v\n", err)
		}
	}()

	if _, err := io.Copy(fp, gzipReader); err != nil {
		return fmt.Errorf("copy to destination file: %v", err)
	}

	if err := fp.Chmod(d.Mode); err != nil {
		return fmt.Errorf("set destination file permissions: %v", err)
	}

	return nil
}

func (d DownloadInfo) receiveFileUncompressed() error {
	fp, err := os.Create(d.Filename)

	if err != nil {
		return fmt.Errorf("create file: %v", err)
	}

	defer func() {
		if err := fp.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error closing file: %v\n", err)
		}
	}()

	if err := fp.Truncate(int64(d.Size)); err != nil {
		return fmt.Errorf("set file size: %v", err)
	}

	if _, err := io.Copy(fp, d.Contents); err != nil {
		return fmt.Errorf("copy to destination file: %v", err)
	}

	if err := fp.Chmod(d.Mode); err != nil {
		return fmt.Errorf("set destination file permissions: %v", err)
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
