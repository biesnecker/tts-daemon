package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	pb "com.biesnecker/tts-daemon/proto"
	"com.biesnecker/tts-daemon/internal/config"
	"com.biesnecker/tts-daemon/internal/daemon"
	"com.biesnecker/tts-daemon/internal/tts"
	"google.golang.org/grpc"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to configuration file (default: ~/.tts-daemon/config.yaml)")
	flag.Parse()

	// Load configuration
	var cfg *config.Config
	var err error

	if *configPath == "" {
		defaultPath, err := config.GetDefaultConfigPath()
		if err != nil {
			log.Fatalf("Failed to get default config path: %v", err)
		}
		*configPath = defaultPath
	}

	cfg, err = config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration from %s: %v", *configPath, err)
	}

	log.Printf("Configuration loaded from %s", *configPath)
	log.Printf("Azure: region=%s, rate_limit=%.1fqps", cfg.Azure.Region, cfg.Azure.MaxQPS)
	log.Printf("Cache: path=%s", cfg.Database.Path)
	log.Printf("Cache: compression=%v", cfg.Database.Compression)
	if cfg.Database.MaxSizeMB > 0 {
		log.Printf("Cache: LRU eviction enabled, max_size=%dMB", cfg.Database.MaxSizeMB)
	} else {
		log.Printf("Cache: LRU eviction disabled (unlimited size)")
	}
	log.Printf("Server: listening on %s:%d", cfg.Server.Address, cfg.Server.Port)

	// Initialize cache
	cache, err := tts.NewCache(cfg.Database.Path, cfg.Database.Compression, cfg.Database.MaxSizeMB)
	if err != nil {
		log.Fatalf("Failed to initialize cache: %v", err)
	}
	defer cache.Close()

	// Print cache stats
	stats, err := cache.GetStats()
	if err != nil {
		log.Printf("Warning: cache stats unavailable: %v", err)
	} else {
		if cfg.Database.MaxSizeMB > 0 {
			log.Printf("Cache: %d entries, %.2fMB/%.2fMB (%.0f%% used)",
				stats["total_clips"], stats["size_mb"], stats["max_size_mb"], stats["usage_percent"])
		} else {
			log.Printf("Cache: %d entries, %.2fMB", stats["total_clips"], stats["size_mb"])
		}
	}

	// Initialize Azure TTS client with rate limiting
	azureClient := tts.NewAzureClient(cfg.Azure.SubscriptionKey, cfg.Azure.Region, cfg.Azure.MaxQPS, cfg.Azure.Voices)
	if len(cfg.Azure.Voices) > 0 {
		log.Printf("Azure: custom voice mappings configured:")
		for locale, voice := range cfg.Azure.Voices {
			log.Printf("  %s -> %s", locale, voice)
		}
	}

	// Fetch available voices from Azure
	log.Printf("Fetching available voices from Azure...")
	if err := azureClient.FetchVoiceList(); err != nil {
		log.Fatalf("Failed to fetch voice list from Azure: %v", err)
	}

	// Initialize TTS service
	ttsService := tts.NewService(cache, azureClient)
	defer ttsService.Close()

	// Create gRPC server
	grpcServer := grpc.NewServer()
	ttsServer := daemon.NewServer(ttsService)
	pb.RegisterTTSServiceServer(grpcServer, ttsServer)

	// Start listening
	address := fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.Port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", address, err)
	}

	log.Printf("Daemon started successfully")

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutdown signal received, stopping...")
		grpcServer.GracefulStop()
	}()

	// Start serving
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
