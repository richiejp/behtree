package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/richiejp/behtree/internal/galcheck"
)

func runReview() {
	flags := flag.NewFlagSet("review", flag.ExitOnError)
	dir := flags.String("dir", "results", "Directory containing report JSON files")
	addr := flags.String("addr", ":8642", "Listen address")

	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}

	server, err := galcheck.NewReviewServer(*dir)
	if err != nil {
		log.Fatalf("review: %v", err)
	}

	fmt.Printf("Review server: http://%s (%d reports loaded from %s)\n", *addr, server.ReportCount(), *dir)
	log.Fatal(http.ListenAndServe(*addr, server.Handler()))
}
