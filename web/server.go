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
	var session common.Session
	var response []byte
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

		fmt.Printf("I just got: %s\n", message)

		commandRaw, argsRaw, _ := bytes.Cut(messageRaw, []byte(" "))

		switch string(commandRaw) {
		case "connect":
			session, err = connect(argsRaw)

			if err != nil {
				response = []byte(err.Error())
			} else {
				response = []byte("connected")

				defer func() {
					if err := session.Session.Signal(ssh.SIGQUIT); err != nil {
						fmt.Printf("send SIGQUIT: %v\n", err)
					}

					if err := session.Session.Close(); err != nil && err != io.EOF {
						fmt.Printf("close session: %v\n", err)
					}
				}()
			}
		case "disconnect":
			session.Session.Signal(ssh.SIGQUIT)
			session.Session.Close()

			response = []byte("disconnected")
		case "list":
			entries, err := list(session)

			if err != nil {
				response = []byte(err.Error())
			} else {
				body, err := json.Marshal(entries)

				if err != nil {
					response = []byte(fmt.Sprintf("marshal dir entries: %v", err))
				} else {
					response = append([]byte("list "), body...)
				}
			}
		case "download":
			filename, fileContents, err := download(session, argsRaw)

			if err != nil {
				response = []byte(err.Error())
			} else {
				data := base64.StdEncoding.EncodeToString(fileContents)
				response = append([]byte(fmt.Sprintf("download %s ", filename)), []byte(data)...)
			}
		case "enter":
			err = enter(session, argsRaw)

			if err != nil {
				response = []byte(err.Error())
			} else {
				response = []byte("entered")
			}
		case "debug":
			stdout, _ := io.ReadAll(session.Stdout)
			stderr, _ := io.ReadAll(session.Stderr)

			response = []byte(fmt.Sprintf("OUT: %s\nERR: %s\n", stdout, stderr))
		default:
			response = []byte(fmt.Sprintf("? %s", message))
		}

		fmt.Printf("I will now respond\n")

		if err := c.WriteMessage(mt, response); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, "error writing websocket message: %v", err)
			return
		}
	}
}

func connect(args []byte) (common.Session, error) {
	var session common.Session
	var request RequestConnection

	if err := json.Unmarshal(args, &request); err != nil {
		return session, fmt.Errorf("unmarshal request body: %v", err)
	}

	info, err := common.ParseConnectionString(request.Hostname)

	if err != nil {
		return session, fmt.Errorf("parse connection string: %v", err)
	}

	remoteClient, err := common.CreateClient(*info)

	if err != nil {
		return session, fmt.Errorf("create client: %v", err)
	}

	if request.Executable == "" {
		executable, err := common.FindExecutable(remoteClient, "qcp")

		if err != nil {
			return session, fmt.Errorf("find executable: %v", err)
		}

		request.Executable = executable
	}

	serveCmd := fmt.Sprintf("%s present %s", request.Executable, request.Location)
	session, err = common.Start(remoteClient, serveCmd)

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

func download(session common.Session, args []byte) (string, []byte, error) {
	var request common.ThinDirEntry

	if err := json.Unmarshal(args, &request); err != nil {
		return "", nil, fmt.Errorf("unmarshal request body: %v", err)
	}

	if _, err := session.Stdin.Write([]byte{protocol.Select}); err != nil {
		return "", nil, err
	}

	if _, err := session.Stdin.Write([]byte(request.Name)); err != nil {
		return "", nil, err
	}

	if _, err := session.Stdin.Write([]byte{protocol.EndTransmission}); err != nil {
		return "", nil, err
	}

	if request.Mode.IsDir() {
		// This whole thing is a nasty process involving lots of
		// buffer re-allocs and magic numbers.

		// The problem is that we are streaming gzipped data
		// without the intention of decompressing it.

		// That's a problem because the only way to determine if
		// we're done reading gzipped data is by decompressing it.

		// This would remove all the benefit of streaming the data.

		filename := request.Name + ".tar.gz"
		rr := bufio.NewReader(session.Stdout)
		fileContents := make([]byte, 0, 1024*1024)

		for {
			chunk, err := rr.ReadBytes(protocol.TerminationSequence[0])

			if err != nil {
				return filename, nil, fmt.Errorf("read: %v", err)
			}

			fileContents = append(fileContents, chunk...)

			for i := 1; i < len(protocol.TerminationSequence); i++ {
				b, err := rr.ReadByte()

				if err != nil {
					return filename, nil, fmt.Errorf("read %v", err)
				}

				fileContents = append(fileContents, b)

				if b != protocol.TerminationSequence[i] {
					break
				}

				if i == len(protocol.TerminationSequence)-1 {
					return filename, fileContents[:len(fileContents)-len(protocol.TerminationSequence)], nil
				}
			}
		}
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

func enter(session common.Session, args []byte) error {
	var request common.ThinDirEntry

	if err := json.Unmarshal(args, &request); err != nil {
		return fmt.Errorf("unmarshal request body: %v", err)
	}

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
