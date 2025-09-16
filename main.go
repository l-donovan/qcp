package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/l-donovan/qcp/common"
	"github.com/l-donovan/qcp/protocol"
	"github.com/l-donovan/qcp/serve"
	"github.com/l-donovan/qcp/sessions"
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
	args := protocol.Parser.MustParseArgs()

	switch args["mode"].(string) {
	case "download":
		connectionString := args["hostname"].(string)
		srcFilePaths := args["sources"].([]string)
		dstFilePath := args["destination"].(string)

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

		if err := sessions.Download(remoteClient, srcFilePaths, dstFilePath); err != nil {
			exitWithError(err)
		}
	case "serve":
		srcFilePaths := args["sources"].([]string)
		offsetFile := args["offset-file"].(string)
		offsetPosStr := args["offset-pos"].(string)

		offsetPos, err := strconv.ParseInt(offsetPosStr, 10, 64)

		if err != nil {
			exitWithError(err)
		}

		// TODO: We get different output here depending on if we use ./my_file or just my_file.
		// Both work, but we want repeatability here.

		uploadInfo := serve.UploadInfo{
			Filenames:   srcFilePaths,
			Destination: os.Stdout,

			// These values may be irrelevant, depending on the input.
			OffsetFile: offsetFile,
			OffsetPos:  offsetPos,
		}

		if err := uploadInfo.Serve(); err != nil {
			exitWithError(err)
		}
	case "upload":
		srcFilePath := args["source"].(string)
		connectionString := args["hostname"].(string)
		dstFilePath := args["destination"].(string)

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

		if err := sessions.Upload(remoteClient, srcFilePath, dstFilePath); err != nil {
			exitWithError(err)
		}
	case "receive":
		dstFilePath := args["destination"].(string)

		downloadInfo, err := sessions.GetDownloadInfo(dstFilePath, os.Stdin)

		if err != nil {
			exitWithError(err)
		}

		// TODO: Partial uploads?
		// Could be tricky because the client initiating the upload would first need to check
		// with the remote to see if there are is a .progress file.
		// It would then use that to determine offset parameters before serving.

		if err := downloadInfo.Receive(nil); err != nil {
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
		quiet := args["quiet"].(bool)

		filename := common.CreateIdentifier(srcFilePaths)

		var downloadInfo serve.DownloadInfo

		// Local connection
		if connectionString == "" {
			readEnd, writeEnd, err := os.Pipe()

			if err != nil {
				exitWithError(err)
			}

			// When sharing a local file, we create our own UploadInfo.

			uploadInfo := serve.UploadInfo{
				Filenames:   srcFilePaths,
				Destination: writeEnd,
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

			downloadSession, err := sessions.StartDownload(remoteClient, srcFilePaths, "", 0)

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

		if !quiet {
			go downloadInfo.PrintProgressBar()
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
