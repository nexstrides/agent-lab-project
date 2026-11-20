package main

import (
	"embed"
	"example.com/enterprise-action-agent/internal/agent"
	"io/fs"
	"log"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

func main() {
	apiHandler := agent.NewHandler(agent.NewSeedStore())
	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	assets, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(assets)))
	addr := ":8090"
	log.Printf("demo API listening at http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
