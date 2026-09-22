package main

import (
	"flag"
	"log"
	"net/http"

	"resonance-workshop/internal/server"
	"resonance-workshop/internal/store"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:5590", "listen address")
	dbPath := flag.String("db", "workshop.db", "SQLite database path")
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	log.Printf("共振归属工坊 listening on http://%s (db=%s)", *listen, *dbPath)
	log.Fatal(http.ListenAndServe(*listen, server.New(st).Handler()))
}
