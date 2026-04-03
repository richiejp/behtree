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
	autoApprove := flags.Bool("auto-approve", false, "Run conservative auto-approve and exit")

	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}

	if *autoApprove {
		runAutoApprove(*dir)
		return
	}

	server, err := galcheck.NewReviewServer(*dir)
	if err != nil {
		log.Fatalf("review: %v", err)
	}

	fmt.Printf("Review server: http://%s (%d reports loaded from %s)\n", *addr, server.ReportCount(), *dir)
	log.Fatal(http.ListenAndServe(*addr, server.Handler()))
}

func runAutoApprove(dir string) {
	reports, err := galcheck.LoadReports(dir)
	if err != nil {
		log.Fatalf("auto-approve: load reports: %v", err)
	}

	result := galcheck.ConservativeAutoApprove(reports)
	fmt.Printf("Auto-approve: %d approved, %d modified, %d skipped (of %d total)\n",
		result.Approved, result.Modified, result.Skipped, len(reports))

	if result.Approved+result.Modified == 0 {
		return
	}

	written := 0
	for _, r := range reports {
		if !galcheck.HasReviewData(r) {
			continue
		}
		if err := galcheck.WriteReportFiles(dir, r); err != nil {
			log.Printf("auto-approve: write %s: %v", r.Name, err)
			continue
		}
		written++
	}
	fmt.Printf("Wrote %d report files to %s\n", written, dir)
}
