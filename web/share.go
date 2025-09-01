package web

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/l-donovan/qcp/serve"
)

type ShareHandler struct {
	mux          *http.ServeMux
	downloadInfo serve.DownloadInfo
	downloadId   string
	server       *http.Server
}

func NewShareHandler(downloadInfo serve.DownloadInfo, server *http.Server) (ShareHandler, error) {
	h := ShareHandler{
		downloadInfo: downloadInfo,
		server:       server,
	}

	id, err := uuid.NewRandom()

	if err != nil {
		return h, fmt.Errorf("create uuid: %w", err)
	}

	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, id.ID())
	b64Id := base64.RawURLEncoding.EncodeToString(bytes)
	h.downloadId = b64Id

	mux := http.NewServeMux()
	mux.HandleFunc("/"+h.downloadId, h.ServeFile)
	h.mux = mux

	return h, nil
}

func (h ShareHandler) ServeFile(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Got download request from %s\n", r.RemoteAddr)
	h.downloadInfo.ReceiveWeb(w)
	fmt.Printf("Done\n")
	go h.server.Shutdown(context.Background())
}

func (h ShareHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h ShareHandler) GetDownloadId() string {
	return h.downloadId
}
