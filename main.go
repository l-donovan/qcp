package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/l-donovan/goparse"
	"github.com/l-donovan/qcp/common"
	"github.com/l-donovan/qcp/receive"
	"github.com/l-donovan/qcp/serve"
	"github.com/l-donovan/qcp/sideload"
	"github.com/l-donovan/qcp/web"
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
			s.AddValueFlag("executable", 'e', "description", "path", "")
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
			s.AddValueFlag("executable", 'e', "description", "path", "")
		},
		"_receive": func(s *goparse.Parser) {
			// Server mode (hidden)
			s.AddParameter("destination", "file to receive")
			s.AddFlag("directory", 'd', "source should be treated as a directory", false)
		},
		"pick": func(s *goparse.Parser) {
			// Client mode
			s.AddParameter("hostname", "connection string, in the format [username@]hostname[:port]")
			s.AddValueFlag("location", 'l', "", "path", "$HOME")
			s.AddValueFlag("executable", 'e', "description", "path", "")
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
			s.AddValueFlag("hostname", 'h', "hostname for web interface", "address", ":8543")
		},
	})

	args := parser.MustParseArgs()

	switch args["mode"].(string) {
	case "download":
		connectionString := args["hostname"].(string)
		srcFilePath := args["source"].(string)
		dstFilePath := args["destination"].(string)
		isDirectory := args["directory"].(bool)
		executable := args["executable"].(string)

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
			err := receive.DownloadDirectory(remoteClient, srcFilePath, dstFilePath, executable)

			if err != nil {
				exitWithError(err)
			}
		} else {
			err = receive.Download(remoteClient, srcFilePath, dstFilePath, executable)

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
		executable := args["executable"].(string)

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
			if err := serve.UploadDirectory(remoteClient, srcFilePath, dstFilePath, executable); err != nil {
				exitWithError(err)
			}
		} else {
			if err := serve.Upload(remoteClient, srcFilePath, dstFilePath, executable); err != nil {
				exitWithError(err)
			}
		}
	case "receive":
		dstFilePath := args["destination"].(string)
		isDirectory := args["directory"].(bool)

		if isDirectory {
			if err := receive.ReceiveDirectory(dstFilePath, os.Stdin, fmt.Printf); err != nil {
				exitWithError(err)
			}
		} else {
			if err := receive.Receive(dstFilePath, os.Stdin); err != nil {
				exitWithError(err)
			}
		}
	case "pick":
		connectionString := args["hostname"].(string)
		location := args["location"].(string)
		executable := args["executable"].(string)

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

		if err := receive.Pick(remoteClient, location, executable); err != nil {
			exitWithError(err)
		}
	case "present":
		location := args["location"].(string)

		err := serve.Present(location, os.Stdin, os.Stdout)

		if err != nil {
			exitWithError(err)
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

		fmt.Printf("Successfully installed \"%s\" on %s at %s\n", release, connectionString, location)
	case "web":
		handler := web.NewHandler()

		server := &http.Server{
			Addr:    ":8543",
			Handler: handler,
		}

		defer handler.CloseClients()

		fmt.Printf("Serving on %s\n", server.Addr)

		if err := server.ListenAndServe(); err != nil {
			exitWithError(err)
		}
	}
}
