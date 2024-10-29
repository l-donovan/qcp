package main

import (
	"fmt"
	"github.com/l-donovan/goparse"
	"github.com/l-donovan/qcp/common"
	"github.com/l-donovan/qcp/receive"
	"github.com/l-donovan/qcp/serve"
	"os"
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
		"_serve": func(s *goparse.Parser) {
			// Server mode (hidden)
			s.AddParameter("source", "file to serve")
		},
		"upload": func(s *goparse.Parser) {
			// Client mode
			s.AddParameter("source", "file to upload")
			s.AddParameter("hostname", "connection string, in the format [username@]hostname[:port]")
			s.AddParameter("destination", "location of uploaded file")
		},
		"_receive": func(s *goparse.Parser) {
			// Server mode (hidden)
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

		info, err := common.ParseConnectionString(connectionString)

		if err != nil {
			panic(err)
		}

		remoteClient, err := common.CreateClient(*info)

		if err != nil {
			panic(err)
		}

		defer func() {
			if err := remoteClient.Close(); err != nil {
				fmt.Printf("error when closing remote client: %v\n", err)
			}
		}()

		if isDirectory {
			err := receive.DownloadDirectory(remoteClient, sourceFilePath, destFilePath)

			if err != nil {
				panic(err)
			}
		} else {
			err = receive.Download(remoteClient, sourceFilePath, destFilePath)

			if err != nil {
				panic(err)
			}
		}
	case "serve":
		sourceFilePath := args["source"].(string)
		isDirectory := args["directory"].(bool)

		if isDirectory {
			err := serve.ServeDirectory(sourceFilePath, os.Stdout)

			if err != nil {
				panic(err)
			}
		} else {
			err := serve.Serve(sourceFilePath, os.Stdout)

			if err != nil {
				panic(err)
			}
		}
	case "upload":
		sourceFilePath := args["source"].(string)
		connectionString := args["hostname"].(string)
		destFilePath := args["destination"].(string)
		isDirectory := args["directory"].(bool)

		info, err := common.ParseConnectionString(connectionString)

		if err != nil {
			panic(err)
		}

		remoteClient, err := common.CreateClient(*info)

		if err != nil {
			panic(err)
		}

		defer func() {
			if err := remoteClient.Close(); err != nil {
				fmt.Printf("error when closing remote client: %v\n", err)
			}
		}()

		if isDirectory {
			err := serve.UploadDirectory(remoteClient, sourceFilePath, destFilePath)

			if err != nil {
				panic(err)
			}
		} else {
			err = serve.Upload(remoteClient, sourceFilePath, destFilePath)

			if err != nil {
				panic(err)
			}
		}
	case "receive":
		destFilePath := args["destination"].(string)
		isDirectory := args["directory"].(bool)

		if isDirectory {
			err := receive.ReceiveDirectory(destFilePath, os.Stdin)

			if err != nil {
				panic(err)
			}
		} else {
			err := receive.Receive(destFilePath, os.Stdin)

			if err != nil {
				panic(err)
			}
		}
	}
}
