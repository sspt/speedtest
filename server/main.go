package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
)

// Global buffer to avoid allocation per download request
var globalBuf = make([]byte, 1024*1024)

func init() {
	// Fill with patterns
	for i := range globalBuf {
		globalBuf[i] = byte(i % 256)
	}
}

// handleDownload streams data to clients for download speed testing
func handleDownload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	// Write loop - no Flush() to allow TCP coalescing
	for {
		if _, err := w.Write(globalBuf); err != nil {
			return
		}
	}
}

// handleUpload accepts data from clients for upload speed testing
func handleUpload(w http.ResponseWriter, r *http.Request) {
	// Efficiently discard the request body
	io.Copy(io.Discard, r.Body)
	w.WriteHeader(http.StatusOK)
}

// handlePing responds immediately for latency testing
func handlePing(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func main() {
	portFlag := flag.Int("port", 8080, "Port to serve on")
	flag.Parse()

	// Check for SERVER_PORT environment variable (takes precedence)
	port := *portFlag
	if envPort := os.Getenv("SERVER_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			port = p
		} else {
			log.Printf("Warning: Invalid SERVER_PORT value '%s', using default %d", envPort, port)
		}
	}

	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.HandleFunc("/download", handleDownload)
	http.HandleFunc("/upload", handleUpload)
	http.HandleFunc("/ping", handlePing)

	log.Printf("HyperSpeed Server running on :%d", port)
	log.Printf("Web UI: http://localhost:%d", port)
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
