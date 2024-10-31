package main

import (
	"fmt"
	"os"

	"github.com/l-donovan/goparse"
	"github.com/l-donovan/qcp/common"
	"github.com/l-donovan/qcp/receive"
	"github.com/l-donovan/qcp/serve"
	"github.com/l-donovan/qcp/sideload"
)

func exitWithMessage(format string, a ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}

func exitWithError(err error) {
	exitWithMessage("%v", err)
}

func main() {
	parser := goparse.NewParser()

	parser.Subparse("mode", "mode of operation", map[string]func(parser *goparse.Parser){
		"download": func(s *goparse.Parser) {
			// Client mode
			s.AddParameter("hostname", "connection string, in the format [username@]hostname[:port]")
			s.AddParameter("source", "file to download")
			s.AddParameter("destination", "location of downloaded file")
			s.AddFlag("directory", 'd', "source should be treated as a directory", false)
		},
		"_serve": func(s *goparse.Parser) {
			// Server mode (hidden)
			s.AddParameter("source", "file to serve")
			s.AddFlag("directory", 'd', "source should be treated as a directory", false)
		},
		"upload": func(s *goparse.Parser) {
			// Client mode
			s.AddParameter("source", "file to upload")
			s.AddParameter("hostname", "connection string, in the format [username@]hostname[:port]")
			s.AddParameter("destination", "location of uploaded file")
			s.AddFlag("directory", 'd', "source should be treated as a directory", false)
		},
		"_receive": func(s *goparse.Parser) {
			// Server mode (hidden)
			s.AddParameter("destination", "file to receive")
			s.AddFlag("directory", 'd', "source should be treated as a directory", false)
		},
		"sideload": func(s *goparse.Parser) {
			// Client mode
			s.AddParameter("hostname", "connection string, in the format [username@]hostname[:port]")
			s.AddValueFlag("release", 'r', "qcp release to sideload (default \"latest\")", "version", "latest")
			s.AddValueFlag("location", 'l', "target location for qcp executable on host (default \"$HOME/bin/qcp\")", "path", "$HOME/bin/qcp")
		},
	})

	args := parser.MustParseArgs()

	switch args["mode"].(string) {
	case "download":
		connectionString := args["hostname"].(string)
		srcFilePath := args["source"].(string)
		dstFilePath := args["destination"].(string)
		isDirectory := args["directory"].(bool)

		info, err := common.ParseConnectionString(connectionString)

		if err != nil {
			exitWithError(err)
		}

		remoteClient, err := common.CreateClient(*info)

		if err != nil {
			exitWithError(err)
		}

		defer func() {
			if err := remoteClient.Close(); err != nil {
				exitWithMessage("error when closing remote client: %v\n", err)
			}
		}()

		if isDirectory {
			err := receive.DownloadDirectory(remoteClient, srcFilePath, dstFilePath)

			if err != nil {
				exitWithError(err)
			}
		} else {
			err = receive.Download(remoteClient, srcFilePath, dstFilePath)

			if err != nil {
				exitWithError(err)
			}
		}
	case "serve":
		srcFilePath := args["source"].(string)
		isDirectory := args["directory"].(bool)

		if isDirectory {
			err := serve.ServeDirectory(srcFilePath, os.Stdout)

			if err != nil {
				exitWithError(err)
			}
		} else {
			err := serve.Serve(srcFilePath, os.Stdout)

			if err != nil {
				exitWithError(err)
			}
		}
	case "upload":
		srcFilePath := args["source"].(string)
		connectionString := args["hostname"].(string)
		dstFilePath := args["destination"].(string)
		isDirectory := args["directory"].(bool)

		info, err := common.ParseConnectionString(connectionString)

		if err != nil {
			exitWithError(err)
		}

		remoteClient, err := common.CreateClient(*info)

		if err != nil {
			exitWithError(err)
		}

		defer func() {
			if err := remoteClient.Close(); err != nil {
				exitWithMessage("error when closing remote client: %v\n", err)
			}
		}()

		if isDirectory {
			err := serve.UploadDirectory(remoteClient, srcFilePath, dstFilePath)

			if err != nil {
				exitWithError(err)
			}
		} else {
			err = serve.Upload(remoteClient, srcFilePath, dstFilePath)

			if err != nil {
				exitWithError(err)
			}
		}
	case "receive":
		dstFilePath := args["destination"].(string)
		isDirectory := args["directory"].(bool)

		if isDirectory {
			err := receive.ReceiveDirectory(dstFilePath, os.Stdin)

			if err != nil {
				exitWithError(err)
			}
		} else {
			err := receive.Receive(dstFilePath, os.Stdin)

			if err != nil {
				exitWithError(err)
			}
		}
	case "sideload":
		connectionString := args["hostname"].(string)
		release := args["release"].(string)
		location := args["location"].(string)

		info, err := common.ParseConnectionString(connectionString)

		if err != nil {
			exitWithError(err)
		}

		remoteClient, err := common.CreateClient(*info)

		if err != nil {
			exitWithError(err)
		}

		defer func() {
			if err := remoteClient.Close(); err != nil {
				exitWithMessage("error when closing remote client: %v\n", err)
			}
		}()

		err = sideload.GetBinary(remoteClient, release, location)

		if err != nil {
			exitWithError(err)
		}
	}
}
