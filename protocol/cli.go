package protocol

import "github.com/l-donovan/goparse"

var (
	Parser goparse.Parser
)

func init() {
	Parser = goparse.NewParser()

	Parser.Subparse("mode", "mode of operation", map[string]func(parser *goparse.Parser){
		"download": func(s *goparse.Parser) {
			// Client mode
			s.AddParameter("hostname", "connection string, in the format [username@]hostname[:port]")
			s.SetListParameter("sources", "files/directories to download", 1)
			s.AddValueFlag("destination", 'd', "location of downloaded file", "PATH", "")
			s.AddFlag("uncompressed", 'u', "source should be uncompressed (parameter has no effect for directory sources)", false)
		},
		"_serve": func(s *goparse.Parser) {
			// Server mode (hidden)
			s.SetListParameter("sources", "files/directories to serve", 1)
			s.AddFlag("uncompressed", 'u', "source should be uncompressed (parameter has no effect for directory sources)", false)
		},
		"upload": func(s *goparse.Parser) {
			// Client mode
			s.AddParameter("source", "file to upload")
			s.AddParameter("hostname", "connection string, in the format [username@]hostname[:port]")
			s.AddParameter("destination", "location of uploaded file")
			s.AddFlag("uncompressed", 'u', "source should be uncompressed (parameter has no effect for directory sources)", false)
		},
		"_receive": func(s *goparse.Parser) {
			// Server mode (hidden)
			s.AddParameter("destination", "file to receive")
		},
		"pick": func(s *goparse.Parser) {
			// Client mode
			s.AddParameter("hostname", "connection string, in the format [username@]hostname[:port]")
			s.AddValueFlag("location", 'l', "", "path", "$HOME")
		},
		"_present": func(s *goparse.Parser) {
			// Server mode (hidden)
			s.AddParameter("location", "")
		},
		"sideload": func(s *goparse.Parser) {
			// Client mode
			s.AddParameter("hostname", "connection string, in the format [username@]hostname[:port]")
			s.AddValueFlag("release", 'r', "qcp release to sideload", "version", "latest")
			s.AddValueFlag("location", 'l', "target location for qcp executable on host", "path", "$HOME/bin/qcp")
		},
		"web": func(s *goparse.Parser) {
			// Web interface mode
			s.AddValueFlag("hostname", 's', "hostname for web interface", "address", ":8543")
		},
		"share": func(s *goparse.Parser) {
			// Link sharing mode
			s.AddValueFlag("hostname", 's', "connection string, in the format [username@]hostname[:port]", "HOST", "")
			s.SetListParameter("sources", "files/directories to serve", 1)
			s.AddFlag("uncompressed", 'u', "source should be uncompressed (parameter has no effect for directory sources)", false)
			s.AddFlag("quiet", 'q', "progress information will not be printed", false)
		},
	})
}
