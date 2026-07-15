package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/services/server/core"
)

var (
	compactDB      = flag.Bool("compact", false, "Run database compaction to reclaim any potential disk space before starting regular services")
	memoryProfiler = flag.Bool("profile", false, "Run pprof memory profiler enabling memory usage debugging")
	bootstrapSetup = flag.Bool("bootstrap-setup", false, "Run first-time setup server before starting relay services")
	setupHost      = flag.String("setup-host", "127.0.0.1", "Host/interface for first-time setup server")
	setupPort      = flag.Int("setup-port", 11012, "Port for first-time setup server")
)

func init() {
	// Parse command-line flags early
	flag.Parse()

	// Initialize config, logging, and UPnP through the shared relay core
	core.Initialize()
}

func main() {
	// Convert OS kill signals into the core stop channel
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	stop := make(chan struct{})
	go func() {
		<-sigs
		close(stop)
	}()

	if err := core.Run(context.Background(), core.Options{
		CompactDB:      *compactDB,
		MemoryProfiler: *memoryProfiler,
		BootstrapSetup: *bootstrapSetup,
		SetupHost:      *setupHost,
		SetupPort:      *setupPort,
		Stop:           stop,
	}); err != nil {
		logging.Fatalf("Relay exited with error: %v", err)
	}
}
