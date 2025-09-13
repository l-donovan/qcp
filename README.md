# `qcp` — Quick Copy

## What is it?
`qcp` is a remote file copying utility, similar to `scp`.
`qcp` focuses on speed and ergonomics. Most sensible options, such as compression, are enabled by default and do not need to be specified.
It's also committed to not mucking up your local directory with a bunch of copied files when you forget if you're supposed to add a trailing slash.
`qcp` is distributed as a single binary. Remote hosts only need a running SSH server and the `qcp` executable somewhere on the `PATH`.

The `qcp` executable can also be sideloaded via the `sideload` command!

## How do I use it?
### Download a file or directory from a remote host
`qcp download user@host:port /path/to/remote/file`

### Upload a file or directory to a remote host
`qcp upload /path/to/local/file user@host:port /path/to/remote/file`

### Download multiple files or directories from a remote host
`qcp download user@host:port /path/to/remote/file/one /path/to/remote/directory/two`

### Download files or directories to a specified target directory
`qcp download user@host:port -d /path/to/local/directory /path/to/remote/file/one /path/to/remote/directory/two`

### Sideload `qcp` onto a new remote host
`qcp sideload user@host:port`

## Future work
### Wildcard support
`qcp download peachtree /etc/nginx/sites-available/*.conf`
`qcp download peachtree /etc/nginx/sites-available/*.conf -d sites`
