// Standalone signaling server binary for deployments where the
// signaling/rendezvous server runs separately from HiveMind nodes.
//
// Usage:
//
//	go run ./signaling-server --port 7777
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/joaopedro/hivemind/internal/infra"
	"github.com/joaopedro/hivemind/internal/logger"
)

func main() {
	port := flag.Int("port", 7777, "signaling server port")
	flag.Parse()

	logger.Init(logger.LevelInfo)
	logger.Info("starting standalone signaling server", "port", *port)

	srv := infra.NewSignalingServer(*port)
	if err := srv.Start(nil); err != nil {
		fmt.Fprintf(os.Stderr, "signaling server error: %v\n", err)
		os.Exit(1)
	}
}
