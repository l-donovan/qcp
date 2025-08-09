package web

type RequestConnection struct {
	Hostname   string `json:"hostname"`
	Location   string `json:"location"`
	Executable string `json:"executable"`
}
