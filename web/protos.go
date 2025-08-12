package web

import "io"

type RequestConnection struct {
	Hostname   string `json:"hostname"`
	Location   string `json:"location"`
	Executable string `json:"executable"`
}

type DownloadInfo struct {
	Filename   string
	Contents   io.Reader
	Compressed bool
}

type HomeInput struct {
	WebsocketEndpoint string
}
