package web

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/l-donovan/qcp/serve"
	"github.com/l-donovan/qcp/sessions"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/l-donovan/qcp/common"
	"golang.org/x/crypto/ssh"
)

var (
	//go:embed "index.gohtml"
	indexHTML string
	tmpl      *template.Template
	upgrader  = websocket.Upgrader{}
)

type Handler struct {
	mux   *http.ServeMux
	files *sync.Map
}

func init() {
	tmpl = template.Must(template.New("index").Parse(indexHTML))
}

func NewHandler() Handler {
	h := Handler{
		files: new(sync.Map),
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
	var session sessions.BrowseSession
	var currentDir string
	var err error

	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "error upgrading websocket connection: %v", err)
		return
	}

	defer func() {
		if err := conn.Close(); err != nil {
		}

		if session != nil {
			session.Stop()
		}
	}()

	for {
		mt, messageRaw, err := conn.ReadMessage()

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

				session, err = sessions.Browse(client, request.Location)

				if err != nil {
					return []byte(err.Error())
				}

				return []byte("connected")
			case "disconnect":
				if session != nil {
					fmt.Print("Disconnecting\n")
					session.Stop()
				}

				return []byte("disconnected")
			case "list":
				fmt.Printf("Listing contents of %s\n", currentDir)

				entries, err := session.ListContents()

				if err != nil {
					return []byte(err.Error())
				}

				body, err := json.Marshal(entries)

				if err != nil {
					return []byte(fmt.Sprintf("marshal dir entries: %v", err))
				}

				return append([]byte("list "), body...)
			case "download":
				var request []common.ThinDirEntry

				if err := json.Unmarshal(argsRaw, &request); err != nil {
					return []byte(err.Error())
				}

				// Downloads get their own session. This way we can use EOF to easily
				// determine when a download stream is completed.

				filepaths := make([]string, len(request))

				for i, item := range request {
					filepaths[i] = path.Join(currentDir, item.Name)
				}

				fmt.Printf("Downloading %s\n", strings.Join(filepaths, ", "))

				downloadSession, err := sessions.StartDownload(client, filepaths, "", 0)

				if err != nil {
					return []byte(err.Error())
				}

				filename := common.CreateIdentifier(filepaths)
				downloadInfo, err := downloadSession.GetDownloadInfo(filename)

				if err != nil {
					return []byte(err.Error())
				}

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

				if err := session.EnterDirectory(request.Name); err != nil {
					return []byte(err.Error())
				}

				currentDir = path.Join(currentDir, request.Name)
				return []byte(fmt.Sprintf("entered %s", currentDir))
			}

			return []byte(fmt.Sprintf("? %s", message))
		}()

		if err := conn.WriteMessage(mt, response); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, "error writing websocket message: %v", err)
			return
		}
	}
}

func createClient(request RequestConnection) (*ssh.Client, error) {
	info, err := common.ParseConnectionString(request.Hostname)

	if err != nil {
		return nil, fmt.Errorf("parse connection string: %w", err)
	}

	remoteClient, err := common.CreateClient(*info)

	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	return remoteClient, nil
}

func (h Handler) ServeFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	downloadInfoRaw, ok := h.files.LoadAndDelete(id)

	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "could not find download with id %s", id)
		return
	}

	downloadInfo := downloadInfoRaw.(serve.DownloadInfo)
	downloadInfo.ReceiveWeb(w)
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
