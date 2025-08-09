package web

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/l-donovan/qcp/common"
	"github.com/l-donovan/qcp/protocol"
	"github.com/l-donovan/qcp/receive"
	"golang.org/x/crypto/ssh"
	"html/template"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	// mux.HandleFunc("/", h.ServeNewSession)
	// mux.HandleFunc("/session/{id}", h.ServeSessionHome)
	// mux.HandleFunc("/session/{id}/connect", h.ServeSessionConnect)
	// mux.HandleFunc("/session/{id}/disconnect", h.ServeSessionDisconnect)
	// mux.HandleFunc("/session/{id}/get-files", h.ServeGetFiles)
	// mux.HandleFunc("/session/{id}/select-file", h.ServeSelectFile)
	// mux.HandleFunc("/session/{id}/enter-directory/{name}", h.ServeEnterDirectory)
	mux.HandleFunc("/session", h.Pick)
	mux.HandleFunc("/", h.ServeHome)
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
		case "select":
			fileContents, err := pick(session, argsRaw)

			if err != nil {
				response = []byte(err.Error())
			} else {
				response = []byte("selected")
			}

			fmt.Printf("File contents: %v\n", fileContents)
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
			response = []byte(fmt.Sprintf(">>>%s<<<", message))
		}

		fmt.Printf("I will now write: %#v\n", response)

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

func splitMagicTerminationSequence(data []byte, atEOF bool) (int, []byte, error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	if i := bytes.Index(data, protocol.TerminationSequence); i >= 0 {
		// We have a full newline-terminated line.
		return i + 2, data[:i], nil
	}

	// If we're at EOF, we have a final, non-terminated line. Return it.
	if atEOF {
		fmt.Printf("INCOMPLETE\n")
		return len(data), data, nil
	}

	// Request more data.
	return 0, nil, nil
}

func pick(session common.Session, args []byte) ([]byte, error) {
	var request common.ThinDirEntry

	if err := json.Unmarshal(args, &request); err != nil {
		return nil, fmt.Errorf("unmarshal request body: %v", err)
	}

	fmt.Printf("Got: %#v\n", request)

	if _, err := session.Stdin.Write([]byte{protocol.Select}); err != nil {
		return nil, err
	}

	if _, err := session.Stdin.Write([]byte(request.Name)); err != nil {
		return nil, err
	}

	if _, err := session.Stdin.Write([]byte{protocol.EndTransmission}); err != nil {
		return nil, err
	}

	if request.Mode.IsDir() {
		fmt.Printf("Reading directory\n")
		scanner := bufio.NewScanner(session.Stdout)
		scanner.Split(splitMagicTerminationSequence)

		for scanner.Scan() {
			return scanner.Bytes(), nil
		}

		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("scan for termination sequence: %v", err)
		}
	} else {
		fmt.Printf("Reading file\n")
		srcReader := bufio.NewReader(session.Stdout)

		fileSizeStr, err := srcReader.ReadString('\n')

		if err != nil {
			return nil, err
		}

		fileSize, err := strconv.Atoi(strings.TrimSpace(fileSizeStr))

		if err != nil {
			return nil, err
		}

		fileModeStr, err := srcReader.ReadString('\n')

		if err != nil {
			return nil, err
		}

		// I don't think we can actually do anything meaningful with the file mode here.
		_, err = strconv.Atoi(strings.TrimSpace(fileModeStr))

		if err != nil {
			return nil, err
		}

		fileContents := make([]byte, fileSize)

		if _, err := io.ReadFull(srcReader, fileContents); err != nil {
			return nil, err
		}

		return fileContents, nil
	}

	return nil, nil
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

func (h Handler) ServeNewSession(w http.ResponseWriter, r *http.Request) {
	id := uuid.New()
	http.Redirect(w, r, fmt.Sprintf("/session/%s", id.String()), http.StatusMovedPermanently)
}

func (h Handler) ServeSessionHome(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, _ = fmt.Fprintf(w, "yippee: %s", id)
}

func (h Handler) ServeSessionDisconnect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	exitChan, exists := h.exit[id]

	if !exists {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "handles do not exist for %s", id)
		return
	}

	exitChan <- struct{}{}
}

func (h *Handler) pick(id string, stdin io.WriteCloser, stdout io.Reader) error {
	// Register our file handles
	h.handles[id] = files{stdin, stdout}
	h.exit[id] = make(chan struct{})

	// Don't do anything until we get the exit signal
	<-h.exit[id]

	// Unregister our handles
	delete(h.handles, id)
	delete(h.exit, id)

	// Make sure we clean up after ourselves
	if _, err := stdin.Write([]byte{protocol.Quit}); err != nil && err != io.EOF {
		_, _ = fmt.Fprintf(os.Stderr, "error when sending quit message to qcp process on remote host: %v\n", err)
		return err
	}

	return nil
}

func (h Handler) ServeGetFiles(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	handles, exists := h.handles[id]

	if !exists {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "handles do not exist for %s", id)
		return
	}

	srcReader := bufio.NewReader(handles.Stdout)

	// List files
	if _, err := handles.Stdin.Write([]byte{protocol.ListFiles}); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "error sending list files command: %v", err)
		return
	}

	// Get output
	result, err := srcReader.ReadString(protocol.EndTransmission)

	// We don't expect an EOF here, so we treat it as a normal error
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "error reading list files output: %v", err)
		return
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
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, "error deserializing dir entry: %v", err)
			return
		}

		entries = append(entries, *entry)
	}

	body, err := json.Marshal(entries)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "error marshaling dir entries: %v", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h Handler) ServeSelectFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var request common.ThinDirEntry

	body, err := io.ReadAll(r.Body)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "failed to read request body: %v", err)
		return
	}

	if err := json.Unmarshal(body, &request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "failed to unmarshal request body: %v", err)
		return
	}

	handles, exists := h.handles[id]

	if !exists {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "handles do not exist for %s", id)
		return
	}

	if _, err := handles.Stdin.Write([]byte{protocol.Select}); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "error sending select file command: %v", err)
		return
	}

	if _, err := handles.Stdin.Write([]byte(request.Name)); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "error sending filename: %v", err)
		return
	}

	if _, err := handles.Stdin.Write([]byte{protocol.EndTransmission}); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "error sending end transmission: %v", err)
		return
	}

	if request.Mode.IsDir() {
		err = receive.ReceiveDirectory(request.Name, handles.Stdout, func(format string, a ...any) (n int, err error) {
			// TODO: we want to log the output somewhere
			return 0, nil
		})
	} else {
		err = receive.Receive(request.Name, handles.Stdout)
	}

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "error receiving file: %v", err)
		return
	}

	// body, err := json.Marshal(entries)

	// if err != nil {
	// 	w.WriteHeader(http.StatusInternalServerError)
	// 	_, _ = fmt.Fprintf(w, "error marshaling dir entries: %v", err)
	// 	return
	// }

	// w.WriteHeader(http.StatusOK)
	// _, _ = w.Write(body)
}

func (h Handler) ServeEnterDirectory(w http.ResponseWriter, r *http.Request) {
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
