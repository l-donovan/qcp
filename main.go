package main

import (
	"os/user"

	"github.com/l-donovan/goparse"
	"github.com/l-donovan/qcp/client"
	"github.com/l-donovan/qcp/server"
)

func main() {
	parser := goparse.NewParser()

	parser.AddFlag("directory", 'd', "source should be treated as a directory", false)
	parser.Subparse("mode", "mode of operation", map[string]func(parser *goparse.Parser){
		"download": func(s *goparse.Parser) {
			// Client mode
			s.AddParameter("hostname", "connection string, in the format [username@]hostname[:port]")
			s.AddParameter("source", "file to download")
			s.AddParameter("destination", "location of downloaded file")
		},
		"serve": func(s *goparse.Parser) {
			// Server mode
			s.AddParameter("source", "file to serve")
		},
		"upload": func(s *goparse.Parser) {
			// Client mode
			// TODO: Implement
			s.AddParameter("source", "file to upload")
			s.AddParameter("hostname", "connection string, in the format [username@]hostname[:port]")
			s.AddParameter("destination", "location of uploaded file")
		},
		"receive": func(s *goparse.Parser) {
			// Server mode
			// TODO: Implement
			s.AddParameter("destination", "file to receive")
		},
	})

	args := parser.MustParseArgs()

	switch args["mode"].(string) {
	case "download":
		connectionString := args["hostname"].(string)
		sourceFilePath := args["source"].(string)
		destFilePath := args["destination"].(string)
		isDirectory := args["directory"].(bool)

		currentUser, err := user.Current()

		if err != nil {
			panic(err)
		}

		info, err := client.ParseConnectionString(connectionString, currentUser.Username, 22)

		if err != nil {
			panic(err)
		}

		// TODO: This obviously shouldn't be hardcoded
		// How do we read from .ssh/config?
		info.PrivateKeyPath = "/Users/ldonovan/.ssh/id_rsa"

		remoteClient, err := client.CreateClient(*info)

		if err != nil {
			panic(err)
		}

		defer remoteClient.Close()

		if isDirectory {
			err := client.DownloadDirectory(remoteClient, sourceFilePath, destFilePath)

			if err != nil {
				panic(err)
			}
		} else {
			err = client.Download(remoteClient, sourceFilePath, destFilePath)

			if err != nil {
				panic(err)
			}
		}

	case "serve":
		sourceFilePath := args["source"].(string)
		isDirectory := args["directory"].(bool)

		if isDirectory {
			err := server.ServeDirectory(sourceFilePath)

			if err != nil {
				panic(err)
			}
		} else {
			err := server.Serve(sourceFilePath)

			if err != nil {
				panic(err)
			}
		}

	case "upload":
	case "receive":

	}
}
