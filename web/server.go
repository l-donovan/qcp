package web

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/l-donovan/qcp/common"
	"github.com/l-donovan/qcp/protocol"
	"golang.org/x/crypto/ssh"
)

//go:embed "index.gohtml"
var indexHTML string

type files struct {
	Stdin  io.WriteCloser
	Stdout io.Reader
}

type Handler struct {
	mux     *http.ServeMux
	clients map[string]*ssh.Client
	handles map[string]files
	exit    map[string]chan struct{}
}

type key int

var clientKey key

func GetClient(ctx context.Context) *ssh.Client {
	return ctx.Value(clientKey).(*ssh.Client)
}

func SetClient(ctx context.Context, client *ssh.Client) context.Context {
	return context.WithValue(ctx, clientKey, client)
}

var (
	tmpl *template.Template
)

func init() {
	tmpl = template.Must(template.New("index").Parse(indexHTML))
}

func NewHandler() Handler {
	h := Handler{
		clients: map[string]*ssh.Client{},
		handles: map[string]files{},
		exit:    map[string]chan struct{}{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", h.ServeHome)
	mux.HandleFunc("/session", h.Pick)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	h.mux = mux

	return h
}

var upgrader = websocket.Upgrader{}

func (h Handler) Pick(w http.ResponseWriter, r *http.Request) {
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
				cmd := fmt.Sprintf("%s serve %s", executable, filepath)

				if request.Mode.IsDir() {
					cmd += " -d"
				}

				downloadSession, err := startSession(client, cmd)

				if err != nil {
					return []byte(err.Error())
				}

				filename, fileContents, err := download(downloadSession, request)

				if err != nil {
					return []byte(err.Error())
				}

				data := base64.StdEncoding.EncodeToString(fileContents)
				return append([]byte(fmt.Sprintf("download %s ", filename)), []byte(data)...)
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
				return []byte("entered")
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

func download(session common.Session, request common.ThinDirEntry) (string, []byte, error) {
	if request.Mode.IsDir() {
		// We should move away from sending files over the websocket
		// connection. In addition to very much not being what websockets
		// are designed for, we miss out on lots of handy stuff
		// like support for partial downloads.

		// I'm thinking we could read the file and then create a one-time
		// download link which we could then send to the client over the
		// websocket.

		filename := request.Name + ".tar.gz"
		fileContents, err := io.ReadAll(session.Stdout)

		return filename, fileContents, err
	} else {
		filename := request.Name
		srcReader := bufio.NewReader(session.Stdout)

		fileSizeStr, err := srcReader.ReadString('\n')

		if err != nil {
			return filename, nil, err
		}

		fileSize, err := strconv.Atoi(strings.TrimSpace(fileSizeStr))

		if err != nil {
			return filename, nil, err
		}

		fileModeStr, err := srcReader.ReadString('\n')

		if err != nil {
			return filename, nil, err
		}

		// I don't think we can actually do anything meaningful with the file mode here.
		_, err = strconv.Atoi(strings.TrimSpace(fileModeStr))

		if err != nil {
			return filename, nil, err
		}

		fileContents := make([]byte, fileSize)

		if _, err := io.ReadFull(srcReader, fileContents); err != nil {
			return filename, nil, err
		}

		return filename, fileContents, nil
	}
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

type HomeInput struct {
	WebsocketEndpoint string
}

func (h Handler) ServeHome(w http.ResponseWriter, r *http.Request) {
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
	for id, client := range h.clients {
		if exitChan, exists := h.exit[id]; exists {
			exitChan <- struct{}{}
		}

		// This is poorly designed and will probably be a race condition

		if err := client.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close connection: %v\n", err)
		} else {
			fmt.Printf("successfully closed connection to %s\n", client.RemoteAddr())
		}
	}
}
