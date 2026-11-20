package main

import (
	"embed"
	"io/fs"
	"log"
	"net"
	"net/http"
	"time"

	"example.com/enterprise-action-agent/internal/agent"
)

//go:embed static/*
var staticFiles embed.FS

const port = "8090"

func main() {
	apiHandler := agent.NewHandler(agent.NewSeedStore())
	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	assets, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(assets)))
	server := &http.Server{
		Handler:           agent.LocalOnly(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// The demo tenant header is not authentication, so the server listens on
	// loopback only and is not reachable from other machines. "localhost"
	// resolves to IPv4 or IPv6 depending on the client, so both are served.
	ipv4, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		log.Fatal(err)
	}
	failed := make(chan error, 2)
	go func() { failed <- server.Serve(ipv4) }()
	if ipv6, err := net.Listen("tcp", "[::1]:"+port); err != nil {
		log.Printf("IPv6 loopback not available, serving IPv4 only: %v", err)
	} else {
		go func() { failed <- server.Serve(ipv6) }()
	}
	log.Printf("demo API listening at http://localhost:%s (loopback only)", port)
	log.Fatal(<-failed)
}
