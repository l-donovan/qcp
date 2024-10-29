# `qcp` — Quick Copy

## What is it?
`qcp` is a remote file copying utility, similar to `scp`.
`qcp` is distributed as a single binary. Remote hosts only need a running SSH server and the `qcp` executable somewhere on the `PATH`.

## How do I use it?
### Download a file from a remote host
`qcp download user@host:port /path/to/remote/file /path/to/local/file`
### Upload a file to a remote host
`qcp upload /path/to/local/file user@host:port /path/to/remote/file`
### Download a directory from a remote host
`qcp -d download user@host:port /path/to/remote/directory /path/to/local/directory`
### Upload a directory to a remote host
`qcp -d upload /path/to/local/directory user@host:port /path/to/remote/directory`
