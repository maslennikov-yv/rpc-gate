package main

import (
	"crypto/tls"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"streaming-server/pkg/handlers"
	"streaming-server/pkg/middleware"
	"streaming-server/pkg/server"
)

func main() {
	// Configure logging with enhanced options
	kafkaConfig := middleware.LoggingConfig{
		KafkaBrokers:   []string{"localhost:9092"},
		Topic:          "rpc-requests",
		Enabled:        true,
		Level:          middleware.LogLevelInfo,
		Format:         middleware.LogFormatJSON,
		Destination:    middleware.LogDestinationKafka,
		LogSuccessOnly: false, // Log both success and error requests
		BufferSize:     1000,
		FlushInterval:  5 * time.Second,
		ServiceName:    "streaming-server",
		ServiceVersion: "1.0.0",
		ExtraFields: map[string]string{
			"environment": "development",
			"region":      "us-west-2",
		},
	}

	// Create logger with new configuration
	logger, err := middleware.NewLogger(kafkaConfig)
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Load TLS configuration
	var tlsConfig *tls.Config
	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile := os.Getenv("TLS_KEY_FILE")

	if certFile == "" {
		certFile = "./certs/server.crt"
	}
	if keyFile == "" {
		keyFile = "./certs/server.key"
	}

	// Check if certificate files exist
	if _, err := os.Stat(certFile); err == nil {
		if _, err := os.Stat(keyFile); err == nil {
			cert, err := tls.LoadX509KeyPair(certFile, keyFile)
			if err != nil {
				log.Printf("Warning: Failed to load TLS certificates: %v", err)
				log.Println("TLS services will be disabled. Run 'make generate-certs' to create certificates.")
			} else {
				tlsConfig = &tls.Config{
					Certificates: []tls.Certificate{cert},
				}
				log.Printf("TLS certificates loaded from %s and %s", certFile, keyFile)
			}
		}
	} else {
		log.Println("TLS certificates not found. TLS services will be disabled.")
		log.Println("Run 'make certs' to create certificates for HTTPS, WSS, and TLS support.")
	}

	// Server configuration
	config := server.Config{
		HTTPAddr:     ":8080",
		HTTPSAddr:    ":8443",
		TCPAddr:      ":8081",
		TLSAddr:      ":8444",
		WSAddr:       ":8082",
		WSSAddr:      ":8445",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
		TLSConfig:    tlsConfig,
		ServiceName:  "streaming-server",
		Version:      "1.0.0",
	}

	// Create and configure server
	srv := server.NewServer(config, logger)

	// Register handlers
	srv.RegisterHandler("echo", handlers.EchoHandler)
	srv.RegisterHandler("time", handlers.TimeHandler)
	srv.RegisterHandler("status", handlers.StatusHandler)
	srv.RegisterHandler("calculate", handlers.CalculateHandler)

	// Start server
	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	log.Println("Server started successfully")
	log.Println("Available endpoints:")
	log.Println("  HTTP:               http://localhost:8080/rpc")
	if tlsConfig != nil {
		log.Println("  HTTPS:              https://localhost:8443/rpc")
	} else {
		log.Println("  HTTPS:              [disabled - no certificates]")
	}
	log.Println("  TCP:                localhost:8081")
	if tlsConfig != nil {
		log.Println("  TLS:                localhost:8444")
	} else {
		log.Println("  TLS:                [disabled - no certificates]")
	}
	log.Println("  WebSocket:          ws://localhost:8082/ws")
	if tlsConfig != nil {
		log.Println("  Secure WebSocket:   wss://localhost:8445/wss")
	} else {
		log.Println("  Secure WebSocket:   [disabled - no certificates]")
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server...")
	if err := srv.Stop(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
	log.Println("Server stopped")
}
