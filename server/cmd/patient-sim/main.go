// cmd/patient-sim/main.go
// TellCartman Stage 3 — Synthetic Patient Simulator.
//
// Spins up N virtual patients and streams realistic, randomly-generated
// check-in records to the gRPC server. Each patient's condition drifts over
// time (random walk with occasional rough patches), producing the kind of
// trend data the platform is designed to analyze.
//
// Examples:
//   # 5 patients, one check-in each every 2s, against the live server
//   go run ./cmd/patient-sim --server 18.218.64.156:50051 --patients 5 --interval 2s
//
//   # Send a fixed total of 50 records then stop
//   go run ./cmd/patient-sim --server localhost:50051 --patients 10 --max 50
//
//   # Run forever until Ctrl+C
//   go run ./cmd/patient-sim --server 18.218.64.156:50051 --patients 3

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	tcv1 "github.com/vinceyoumans/aihack26_ch03/server/proto/gen"
)

func main() {
	var (
		server   = flag.String("server", "localhost:50051", "gRPC server address host:port")
		patients = flag.Int("patients", 3, "number of virtual patients to simulate")
		interval = flag.Duration("interval", 2*time.Second, "time between each patient's check-ins")
		maxRecs  = flag.Int("max", 0, "stop after this many total records (0 = run until Ctrl+C)")
		seed     = flag.Int64("seed", time.Now().UnixNano(), "RNG seed for reproducible runs")
		timeout  = flag.Duration("rpc-timeout", 10*time.Second, "per-RPC timeout")
	)
	flag.Parse()

	if *patients < 1 {
		log.Fatal("--patients must be >= 1")
	}

	log.Printf("TellCartman patient simulator")
	log.Printf("  server:   %s", *server)
	log.Printf("  patients: %d", *patients)
	log.Printf("  interval: %s", *interval)
	if *maxRecs > 0 {
		log.Printf("  max recs: %d", *maxRecs)
	} else {
		log.Printf("  max recs: unlimited (Ctrl+C to stop)")
	}

	// ── Connect to the gRPC server ───────────────────────────────────────────
	conn, err := grpc.NewClient(*server, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to create gRPC client: %v", err)
	}
	defer conn.Close()
	client := tcv1.NewPatientDataServiceClient(conn)

	// ── Create the virtual patients ──────────────────────────────────────────
	pats := make([]*VirtualPatient, *patients)
	for i := range pats {
		pats[i] = NewVirtualPatient(*seed + int64(i))
		log.Printf("  created %s (%s)", pats[i].PatientID, pats[i].DisplayName)
	}

	// ── Shutdown handling ────────────────────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Println("shutdown signal received, stopping...")
		cancel()
	}()

	// ── Counters ─────────────────────────────────────────────────────────────
	var sent, failed int64

	// ── One goroutine per patient, each sending on its own ticker ────────────
	var wg sync.WaitGroup
	for _, p := range pats {
		wg.Add(1)
		go func(p *VirtualPatient) {
			defer wg.Done()
			ticker := time.NewTicker(*interval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// Enforce global max across all patients
					if *maxRecs > 0 && atomic.LoadInt64(&sent) >= int64(*maxRecs) {
						cancel()
						return
					}

					rec := p.NextRecord()
					rpcCtx, rpcCancel := context.WithTimeout(ctx, *timeout)
					resp, err := client.SubmitRecord(rpcCtx, rec)
					rpcCancel()

					if err != nil || !resp.GetAccepted() {
						atomic.AddInt64(&failed, 1)
						log.Printf("✗ %s record=%s err=%v", p.PatientID, rec.GetRecordId(), err)
						continue
					}

					n := atomic.AddInt64(&sent, 1)
					// Log a compact summary including the key signals
					log.Printf("✓ #%d %s feeling=%d mental=%s stress_events=%d",
						n, p.PatientID,
						rec.GetBodyStatus().GetGeneralFeeling(),
						rec.GetBodyStatus().GetMentalState(),
						len(rec.GetStressorEvents()),
					)
				}
			}
		}(p)
	}

	wg.Wait()

	fmt.Println()
	log.Printf("done. sent=%d failed=%d", atomic.LoadInt64(&sent), atomic.LoadInt64(&failed))
	if atomic.LoadInt64(&failed) > 0 {
		os.Exit(1)
	}
}
