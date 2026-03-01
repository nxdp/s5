package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	addr     = flag.String("l", "127.0.0.1:1080", "listen address")
	username = flag.String("u", "admin", "username")
	password = flag.String("p", "admin", "password")
	bufsize  = flag.Int("b", 65536, "buffer size in bytes")
	timeout  = flag.Duration("t", 5*time.Second, "connection timeout")
)

// global counter
var activeConns int64

// store pointers to avoid interface allocation
var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, *bufsize)
		return &b
	},
}

func main() {
	flag.Parse()
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("listening on %s", *addr)

	// log status every 10 seconds
	go func() {
		for {
			time.Sleep(10 * time.Second)
			log.Printf("status: %d active connections", atomic.LoadInt64(&activeConns))
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handle(conn)
	}
}

func handle(conn net.Conn) {
	// increment counter when connection starts
	atomic.AddInt64(&activeConns, 1)

	// decrement counter when handle returns
	defer atomic.AddInt64(&activeConns, -1)

	// get first buffer
	bufPtr := bufPool.Get().(*[]byte)
	defer bufPool.Put(bufPtr)
	buf := *bufPtr

	// set deadline for handshake protection
	conn.SetDeadline(time.Now().Add(*timeout))

	// greeting
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		conn.Close()
		return
	}
	nmethods := buf[1]
	if _, err := io.ReadFull(conn, buf[:nmethods]); err != nil {
		conn.Close()
		return
	}
	// require user/pass auth (0x02)
	conn.Write([]byte{0x05, 0x02})

	// auth
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		conn.Close()
		return
	}
	ulen := buf[1]
	if _, err := io.ReadFull(conn, buf[:ulen]); err != nil {
		conn.Close()
		return
	}
	user := string(buf[:ulen])

	if _, err := io.ReadFull(conn, buf[:1]); err != nil {
		conn.Close()
		return
	}
	plen := buf[0]
	if _, err := io.ReadFull(conn, buf[:plen]); err != nil {
		conn.Close()
		return
	}
	pass := string(buf[:plen])

	if user != *username || pass != *password {
		conn.Write([]byte{0x01, 0x01})
		conn.Close()
		return
	}
	conn.Write([]byte{0x01, 0x00})

	// request
	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		conn.Close()
		return
	}
	if buf[1] != 0x01 { // only CONNECT
		conn.Close()
		return
	}

	var target string
	switch buf[3] {
	case 0x01: // ipv4
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			conn.Close()
			return
		}
		target = net.IP(buf[:4]).String()
	case 0x03: // domain
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			conn.Close()
			return
		}
		dlen := buf[0]
		if _, err := io.ReadFull(conn, buf[:dlen]); err != nil {
			conn.Close()
			return
		}
		target = string(buf[:dlen])
	case 0x04: // ipv6
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			conn.Close()
			return
		}
		target = net.IP(buf[:16]).String()
	}

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		conn.Close()
		return
	}
	port := int(buf[0])<<8 | int(buf[1])
	addr := net.JoinHostPort(target, fmt.Sprintf("%d", port))

	dst, err := net.DialTimeout("tcp", addr, *timeout)
	if err != nil {
		conn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		conn.Close()
		return
	}

	conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	// remove deadline so data transfer can flow freely
	conn.SetDeadline(time.Time{})

	// get second buffer for pipe
	bufPtr2 := bufPool.Get().(*[]byte)
	defer bufPool.Put(bufPtr2)
	buf2 := *bufPtr2

	// pipe with sync
	done := make(chan struct{}, 2)

	go func() {
		io.CopyBuffer(dst, conn, buf)
		done <- struct{}{}
	}()

	go func() {
		io.CopyBuffer(conn, dst, buf2)
		done <- struct{}{}
	}()

	// wait for one side to close
	<-done
	conn.Close()
	dst.Close()

	// wait for other side to finish so buffers are safe to return
	<-done
}
