package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/l-donovan/qcp/sessions"

	"github.com/l-donovan/goparse"
	"github.com/l-donovan/qcp/common"
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
			s.AddFlag("uncompressed", 'u', "source should be uncompressed (parameter has no effect for directory sources)", false)
		},
		"_serve": func(s *goparse.Parser) {
			// Server mode (hidden)
			s.AddListParameter("sources", "files/directories to serve", 1)
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
			s.AddValueFlag("hostname", 'h', "hostname for web interface", "address", ":8543")
		},
		"share": func(s *goparse.Parser) {
			// Link sharing mode
			s.AddValueFlag("hostname", 'h', "connection string, in the format [username@]hostname[:port]", "HOST", "")
			s.AddListParameter("sources", "files/directories to serve", 1)
			s.AddFlag("uncompressed", 'u', "source should be uncompressed (parameter has no effect for directory sources)", false)
		},
	})

	args := parser.MustParseArgs()

	switch args["mode"].(string) {
	case "download":
		connectionString := args["hostname"].(string)
		srcFilePath := args["source"].(string)
		dstFilePath := args["destination"].(string)
		uncompressed := args["uncompressed"].(bool)

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

		if err := sessions.Download(remoteClient, []string{srcFilePath}, dstFilePath, !uncompressed); err != nil {
			exitWithError(err)
		}
	case "serve":
		srcFilePaths := args["sources"].([]string)
		uncompressed := args["uncompressed"].(bool)

		fileInfo, err := os.Stat(srcFilePaths[0])

		if err != nil {
			exitWithError(err)
		}

		uploadInfo := serve.UploadInfo{
			Filenames:   srcFilePaths,
			Destination: os.Stdout,

			// These values may be irrelevant, depending on the input.
			Directory:  fileInfo.IsDir(),
			Compressed: !uncompressed,
		}

		if err := uploadInfo.Serve(); err != nil {
			exitWithError(err)
		}
	case "upload":
		srcFilePath := args["source"].(string)
		connectionString := args["hostname"].(string)
		dstFilePath := args["destination"].(string)
		uncompressed := args["uncompressed"].(bool)

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

		if err := sessions.Upload(remoteClient, srcFilePath, dstFilePath, !uncompressed); err != nil {
			exitWithError(err)
		}
	case "receive":
		dstFilePath := args["destination"].(string)

		downloadInfo, err := sessions.GetDownloadInfo(dstFilePath, os.Stdin)

		if err != nil {
			exitWithError(err)
		}

		if err := downloadInfo.Receive(); err != nil {
			exitWithError(err)
		}
	case "pick":
		connectionString := args["hostname"].(string)
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

		if err := sessions.Pick(remoteClient, location); err != nil {
			exitWithError(err)
		}
	case "present":
		location := args["location"].(string)

		browseInfo := serve.BrowseInfo{
			Location:    location,
			Source:      os.Stdin,
			Destination: os.Stdout,
		}

		if err := browseInfo.Present(); err != nil {
			exitWithMessage("present: %v", err)
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

		fmt.Printf("Serving on %s\n", server.Addr)

		if err := server.ListenAndServe(); err != nil {
			exitWithError(err)
		}
	case "share":
		connectionString := args["hostname"].(string)
		srcFilePaths := args["sources"].([]string)
		uncompressed := args["uncompressed"].(bool)

		filename := common.CreateIdentifier(srcFilePaths)

		var downloadInfo serve.DownloadInfo

		// Local connection
		if connectionString == "" {
			readEnd, writeEnd, err := os.Pipe()

			if err != nil {
				exitWithError(err)
			}

			fileInfo, err := os.Stat(srcFilePaths[0])

			if err != nil {
				exitWithError(err)
			}

			// When sharing a local file, we create our own UploadInfo.

			uploadInfo := serve.UploadInfo{
				Filenames:   srcFilePaths,
				Destination: writeEnd,
				Directory:   fileInfo.IsDir(),
				Compressed:  !uncompressed,
			}

			go func() {
				if err := uploadInfo.Serve(); err != nil {
					exitWithError(err)
				}

				if err := writeEnd.Close(); err != nil {
					exitWithError(err)
				}
			}()

			dlInfo, err := sessions.GetDownloadInfo(filename, readEnd)

			if err != nil {
				exitWithError(err)
			}

			downloadInfo = dlInfo
		} else {
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

			downloadSession, err := sessions.StartDownload(remoteClient, srcFilePaths, !uncompressed)

			if err != nil {
				exitWithError(err)
			}

			defer downloadSession.Stop()

			dlInfo, err := downloadSession.GetDownloadInfo(filename)

			if err != nil {
				exitWithError(err)
			}

			downloadInfo = dlInfo
		}

		ip, err := common.GetOutboundIP()

		if err != nil {
			exitWithError(err)
		}

		server := &http.Server{
			Addr: ip.String() + ":8543",
		}

		handler, err := web.NewShareHandler(downloadInfo, server)

		if err != nil {
			exitWithError(err)
		}

		server.Handler = handler

		fmt.Printf("Download link: http://%s/%s\n", server.Addr, handler.GetDownloadId())

		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			exitWithError(err)
		}
	}
}
