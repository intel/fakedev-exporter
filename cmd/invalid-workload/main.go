// Copyright 2022 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
//
// invalid-workload test client sends invalid data to fakedev-exporter server,
// and exits with zero if server returned error code for all.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const readTimeoutMs = 200

// provoke server to process new WL connection by requesting metrics
// TODO: remove once server works on its own clock?
func queryMetrics(url string) {
	resp, err := http.Get(url)
	if err != nil {
		log.Fatalf("ERROR: server metrics query to address '%s' failed with: %v", url, err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Fatalf("ERROR: server metric query response failed with status code %d and\nbody: %s",
			resp.StatusCode, body)
	}
	if err != nil {
		log.Fatalf("ERROR: server metric query response body read error: %v", err)
	}
}

// sendInvalidMsg sends given invalid message to server socket 'path',
// queries metrics 'url' to process it, and waits for a quick reply.
// If server does not return an error code, or anything fails, exit with error
func sendInvalidMsg(path, url string, msg []byte) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		log.Fatalf("ERROR: connection to 'fakedev-exporter' unix socket '%s' failed: %v", path, err)
	}
	defer conn.Close()
	if len(msg) > 30 {
		log.Printf("Invalid message to server:\n%v...", msg[:30])
	} else {
		log.Printf("Invalid message to server:\n%v", msg)
	}
	n, err := conn.Write(msg)
	if err != nil || n != len(msg) {
		log.Fatalf("ERROR: data write (%d/%d bytes) to 'fakedev-exporter' failed: %v", n, len(msg), err)
	}
	// provoke server to process new WL
	queryMetrics(url)
	// wait for server to provide error code
	data := make([]byte, 8)
	conn.SetReadDeadline(time.Now().Add(readTimeoutMs * time.Millisecond))
	n, err = conn.Read(data)
	if err != nil {
		log.Fatalf("ERROR: 'fakedev-exporter' socket %dms read failed: %v", readTimeoutMs, err)
	}
	retval := string(data[:n])
	ret, err := strconv.Atoi(retval)
	if err != nil {
		log.Fatalf("ERROR: could not parse 'fakedev-exporter' exit code '%s': %v", retval, err)
	}
	if ret == 0 {
		log.Fatal("ERROR: server returned zero, not an error code")
	}
	log.Printf("Server returned: %d", ret)
}

func main() {
	var devs, socket, url string
	flag.StringVar(&devs, "devnames", "card0", "Comma separate list of device file names to use in communication")
	flag.StringVar(&socket, "socket", "/tmp/fakedev-exporter", "Unix socket path for workload communication")
	flag.StringVar(&url, "url", "http://127.0.0.1:9999/metrics", "URL where to query resulting metrics")
	flag.Parse()
	devnames := "[\"" + strings.Join(strings.Split(devs, ","), "\",\"") + "\"]"
	log.Printf("Connecting to '%s' socket, prodding '%s' URL, and claiming to have device(s): %v", socket, url, devnames)
	valid := []byte(fmt.Sprintf("{\"Name\":\"Invalid\",\"Devices\":%v,\"Profile\":[{\"Load\":0}],", devnames))
	tests := [][]byte{
		// invalid
		[]byte(""),
		[]byte("foobar"),
		make([]byte, 200),
		// missing members
		[]byte("{\"Foobar\":0}"),
		// invalid repeat count value
		append(valid, []byte("\"Repeat\":-1}")...),
		// invalid load value
		append(valid, []byte("\"Profile\":[{\"Load\":-1}]}")...),
		// invalid load+fluctuation total value
		append(valid, []byte("\"Profile\":[{\"Load\":50,\"Fluctuation\":75}]}")...),
		// invalid large JSON string after valid content
		append([]byte("{\"Name\":\")"), make([]byte, 64*1024)...),
	}
	for _, t := range tests {
		sendInvalidMsg(socket, url, t)
	}
}
