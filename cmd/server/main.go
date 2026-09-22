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
		log.Fatal(err)
	}
	defer st.Close()

	d, err := st.Load()
	if err != nil {
		log.Fatal(err)
	}
	if len(d.Experiments) == 0 {
		log.Println("empty database, seeding fixture")
		if err := st.Seed(); err != nil {
			log.Fatal(err)
		}
	}

	log.Printf("共振归属工坊 listening on http://%s", *listen)
	log.Fatal(http.ListenAndServe(*listen, server.New(st).Handler()))
}
