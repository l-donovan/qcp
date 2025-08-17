package web

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/l-donovan/qcp/common"
	"github.com/l-donovan/qcp/protocol"
	"golang.org/x/crypto/ssh"
)

var (
	//go:embed "index.gohtml"
	indexHTML string
	tmpl      *template.Template
	upgrader  = websocket.Upgrader{}
)

type Handler struct {
	mux     *http.ServeMux
	clients []*ssh.Client
	files   *sync.Map
}

func init() {
	tmpl = template.Must(template.New("index").Parse(indexHTML))
}

func NewHandler() Handler {
	h := Handler{
		clients: []*ssh.Client{},
		files:   new(sync.Map),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", ServeHome)
	mux.HandleFunc("/session", h.ServeSession)
	mux.HandleFunc("/file/{id}", h.ServeFile)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	h.mux = mux

	return h
}

func (h Handler) ServeSession(w http.ResponseWriter, r *http.Request) {
	var client *ssh.Client
	var session common.Session
	var executable string
	var currentDir string
	var err error

	c, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "error upgrading websocket connection: %v", err)
		return
	}

	defer func() {
		if err := c.Close(); err != nil {
		}
	}()

	for {
		mt, messageRaw, err := c.ReadMessage()

		if err != nil {
			var closeErr *websocket.CloseError

			if !errors.As(err, &closeErr) {
				_, _ = fmt.Fprintf(os.Stderr, "Error reading websocket message: %v\n", err)
			}

			return
		}

		message := string(messageRaw)

		commandRaw, argsRaw, _ := bytes.Cut(messageRaw, []byte(" "))

		response := func() []byte {
			switch string(commandRaw) {
			case "connect":
				var request RequestConnection

				if err := json.Unmarshal(argsRaw, &request); err != nil {
					return []byte(err.Error())
				}

				fmt.Printf("Connecting to %s:%s\n", request.Hostname, request.Location)
				currentDir = request.Location

				// New connection means new SSH client.

				client, err = createClient(request)

				if err != nil {
					return []byte(err.Error())
				}

				// New SSH client means we need to find the qcp executable, assuming
				// it hasn't been provided in the request.

				executable, err = findExecutable(client, request)

				if err != nil {
					return []byte(err.Error())
				}

				cmd := fmt.Sprintf("%s present %s", executable, request.Location)
				session, err = startSession(client, cmd)

				if err != nil {
					return []byte(err.Error())
				}

				// TODO: Put this somewhere.
				// defer func() {
				// 	if err := session.Session.Signal(ssh.SIGQUIT); err != nil {
				// 		fmt.Printf("send SIGQUIT: %v\n", err)
				// 	}

				// 	if err := session.Session.Close(); err != nil && err != io.EOF {
				// 		fmt.Printf("close session: %v\n", err)
				// 	}
				// }()

				return []byte("connected")
			case "disconnect":
				fmt.Printf("Disconnecting\n")

				session.Session.Signal(ssh.SIGQUIT)
				session.Session.Close()

				return []byte("disconnected")
			case "list":
				fmt.Printf("Listing contents of %s\n", currentDir)

				entries, err := list(session)

				if err != nil {
					return []byte(err.Error())
				}

				body, err := json.Marshal(entries)

				if err != nil {
					return []byte(fmt.Sprintf("marshal dir entries: %v", err))
				}

				return append([]byte("list "), body...)
			case "download":
				var request common.ThinDirEntry

				if err := json.Unmarshal(argsRaw, &request); err != nil {
					return []byte(err.Error())
				}

				// Downloads get their own session. This way we can use EOF to easily
				// determine when a download stream is completed.

				filepath := path.Join(currentDir, request.Name)
				fmt.Printf("Downloading %s\n", filepath)

				// TODO: We want compression parameterized.
				cmd := fmt.Sprintf("%s serve %s", executable, filepath)

				downloadSession, err := startSession(client, cmd)

				if err != nil {
					return []byte(err.Error())
				}

				downloadInfo, err := download(downloadSession, request)

				if err != nil {
					return []byte(err.Error())
				}

				id, err := uuid.NewRandom()

				if err != nil {
					return []byte(err.Error())
				}

				downloadLink := fmt.Sprintf("/file/%s", id.String())
				h.files.Store(id.String(), downloadInfo)
				fmt.Printf("Created new download link for %s: %s\n", filepath, downloadLink)

				return []byte(fmt.Sprintf("download %s", downloadLink))
			case "download-bulk":
				var request []common.ThinDirEntry

				if err := json.Unmarshal(argsRaw, &request); err != nil {
					return []byte(err.Error())
				}

				filepaths := make([]string, len(request))

				for i, item := range request {
					filepaths[i] = path.Join(currentDir, item.Name)
				}

				fmt.Printf("Downloading %s\n", strings.Join(filepaths, ", "))

				// TODO: We really need some shlex escaping here (and above).
				cmd := fmt.Sprintf("%s serve-multiple %s", executable, strings.Join(filepaths, " "))
				downloadSession, err := startSession(client, cmd)

				if err != nil {
					return []byte(err.Error())
				}

				// Throw away the flag Byte, we already know it'll be 0x01.
				p := make([]byte, 1)
				if _, err := downloadSession.Stdout.Read(p); err != nil {
					return []byte(err.Error())
				}

				filename := createIdentifier(filepaths)

				// TODO: We want compression parameterized.
				downloadInfo := DownloadInfo{Filename: filename + ".tar.gz", Contents: downloadSession.Stdout, Compressed: true}

				id, err := uuid.NewRandom()

				if err != nil {
					return []byte(err.Error())
				}

				downloadLink := fmt.Sprintf("/file/%s", id.String())
				h.files.Store(id.String(), downloadInfo)
				fmt.Printf("Created new download link for %s: %s\n", strings.Join(filepaths, ", "), downloadLink)

				return []byte(fmt.Sprintf("download %s", downloadLink))
			case "enter":
				var request common.ThinDirEntry

				if err := json.Unmarshal(argsRaw, &request); err != nil {
					return []byte(err.Error())
				}

				fmt.Printf("Entering %s\n", request.Name)

				if err := enter(session, request); err != nil {
					return []byte(err.Error())
				}

				currentDir = path.Join(currentDir, request.Name)
				return []byte(fmt.Sprintf("entered %s", currentDir))
			}

			return []byte(fmt.Sprintf("? %s", message))
		}()

		if err := c.WriteMessage(mt, response); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, "error writing websocket message: %v", err)
			return
		}
	}
}

func createIdentifier(names []string) string {
	basenames := make([]string, len(names))

	for i, name := range names {
		basenames[i] = path.Base(name)
	}

	id := strings.Join(basenames, "__")
	maxLen := 50
	andMore := " (...)"

	if len(id) > maxLen {
		id = id[:(maxLen-len(andMore))] + andMore
	}

	return id
}

func createClient(request RequestConnection) (*ssh.Client, error) {
	info, err := common.ParseConnectionString(request.Hostname)

	if err != nil {
		return nil, fmt.Errorf("parse connection string: %v", err)
	}

	remoteClient, err := common.CreateClient(*info)

	if err != nil {
		return nil, fmt.Errorf("create client: %v", err)
	}

	return remoteClient, nil
}

func findExecutable(remoteClient *ssh.Client, request RequestConnection) (string, error) {
	if request.Executable == "" {
		executable, err := common.FindExecutable(remoteClient, "qcp")

		if err != nil {
			return "", fmt.Errorf("find executable: %v", err)
		}

		return executable, nil
	}

	return request.Executable, nil
}

func startSession(remoteClient *ssh.Client, cmd string) (common.Session, error) {
	session, err := common.Start(remoteClient, cmd)

	if err != nil {
		return session, fmt.Errorf("start command: %v", err)
	}

	return session, nil
}

func list(session common.Session) ([]common.ThinDirEntry, error) {
	srcReader := bufio.NewReader(session.Stdout)

	// List files
	if _, err := session.Stdin.Write([]byte{protocol.ListFiles}); err != nil {
		return nil, fmt.Errorf("send list files command: %v", err)
	}

	// Get output
	result, err := srcReader.ReadString(protocol.EndTransmission)

	// We don't expect an EOF here, so we treat it as a normal error
	if err != nil {
		return nil, fmt.Errorf("read list files output: %v", err)
	}

	var entries []common.ThinDirEntry
	serializedEntries := strings.Split(strings.TrimSuffix(result, string(protocol.EndTransmission)), string(protocol.FileSeparator))

	for _, rawEntry := range serializedEntries {
		// This happens in empty directories because strings.Split("", "<separator>") returns []string{""}, not []string{}
		if rawEntry == "" {
			continue
		}

		entry, err := common.DeserializeDirEntry(rawEntry)

		if err != nil {
			return nil, fmt.Errorf("deserialize dir entry: %v", err)
		}

		entries = append(entries, *entry)
	}

	return entries, nil
}

func enter(session common.Session, request common.ThinDirEntry) error {
	if _, err := session.Stdin.Write([]byte{protocol.Enter}); err != nil {
		return err
	}

	if _, err := session.Stdin.Write([]byte(request.Name)); err != nil {
		return err
	}

	if _, err := session.Stdin.Write([]byte{protocol.EndTransmission}); err != nil {
		return err
	}

	return nil
}

func download(session common.Session, request common.ThinDirEntry) (DownloadInfo, error) {
	f := make([]byte, 1)

	if _, err := session.Stdout.Read(f); err != nil {
		return DownloadInfo{}, fmt.Errorf("read flags: %v", err)
	}

	isDir := f[0]&protocol.IsDirectory > 0
	isCompressed := f[0]&protocol.IsCompressed > 0

	if isDir {
		return DownloadInfo{Filename: request.Name + ".tar.gz", Contents: session.Stdout, Compressed: true}, nil
	} else if isCompressed {
		fileModeBytes := make([]byte, 4)

		if _, err := session.Stdout.Read(fileModeBytes); err != nil {
			return DownloadInfo{}, fmt.Errorf("read file mode: %v", err)
		}

		return DownloadInfo{Filename: request.Name, Contents: session.Stdout, Compressed: true}, nil
	} else {
		fileSizeBytes := make([]byte, 4)
		fileModeBytes := make([]byte, 4)

		if _, err := session.Stdout.Read(fileSizeBytes); err != nil {
			return DownloadInfo{}, fmt.Errorf("read file size: %v", err)
		}

		if _, err := session.Stdout.Read(fileModeBytes); err != nil {
			return DownloadInfo{}, fmt.Errorf("read file mode: %v", err)
		}

		return DownloadInfo{Filename: request.Name, Contents: session.Stdout, Compressed: false}, nil
	}
}

func (h Handler) ServeFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	downloadInfoRaw, ok := h.files.LoadAndDelete(id)

	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "could not find download with id %s", id)
		return
	}

	downloadInfo := downloadInfoRaw.(DownloadInfo)
	flusher, ok := w.(http.Flusher)

	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "no good")
		return
	}

	mimeType := mime.TypeByExtension(path.Ext(downloadInfo.Filename))

	if mimeType == "" {
		mimeType = "text/plain"
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "attachment;filename="+downloadInfo.Filename)

	if downloadInfo.Compressed {
		w.Header().Set("Content-Encoding", "gzip")
	}

	for {
		if _, err := io.CopyN(w, downloadInfo.Contents, 1024); err != nil {
			if err != io.EOF {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = fmt.Fprintf(w, "download: %v", err)
			}

			return
		}

		flusher.Flush()
	}
}

func ServeHome(w http.ResponseWriter, r *http.Request) {
	input := HomeInput{
		WebsocketEndpoint: "/session",
	}

	if err := tmpl.Execute(w, input); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "error executing template: %v", err)
		return
	}
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h Handler) CloseClients() {
	for _, client := range h.clients {
		// This is poorly designed and will probably be a race condition

		if err := client.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close connection: %v\n", err)
		} else {
			fmt.Printf("Successfully closed connection to %s\n", client.RemoteAddr())
		}
	}
}
