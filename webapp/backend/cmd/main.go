package main

import (
	"context"
	"log"

	"backend/internal/server"
	"backend/internal/telemetry"
)

func main() {

	// jaeger の初期化
	shutdown, err := telemetry.Init(context.Background())
	if err != nil {
		log.Printf("telemetry init failed: %v, continuing without telemetry", err)
	} else {
		defer func() { _ = shutdown(context.Background()) }()
	}

	srv, dbConn, err := server.NewServer()
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}
	if dbConn != nil {
		defer dbConn.Close()
	}

	srv.Run()
}
