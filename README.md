# S5

A minimal SOCKS5 proxy.

It supports:

* CONNECT command only (TCP)
* Username and password authentication
* IPv4, IPv6 and domain targets
* Configurable timeout and buffer size
* Active connection counter

This project is focused on simplicity and readability.

---

## Build

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
go build -trimpath -ldflags="-s -w -buildid=" -o s5
```

---

## Run

```bash
./s5 [flags]
```

### Flags

```
-l string
    listen address (default "127.0.0.1:1080")

-u string
    username (default "admin")

-p string
    password (default "admin")

-b int
    buffer size in bytes (default 65536)

-t duration
    connection timeout (default 5s)
```

### Example

```bash
./s5 -l 0.0.0.0:1080 -u user -p secret
```

Then configure your client to use:

* SOCKS5
* Host: your server IP
* Port: 1080
* Username and password you set

