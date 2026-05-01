package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/cmd184psu/ctrq/internal/config"
	"github.com/cmd184psu/ctrq/internal/coordinator"
	"github.com/cmd184psu/ctrq/internal/db"
	"github.com/cmd184psu/ctrq/internal/models"
	"github.com/cmd184psu/ctrq/internal/worker"
)

func RunServices() {
	cfgPath := flag.String("config", config.DefaultConfigPath, "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	// Seed groups from config into DB
	if err := seedGroups(database, cfg); err != nil {
		log.Fatalf("seed groups: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	workerID, _ := os.Hostname()
	w := worker.New(database, cfg, workerID)
	worker.Registry.StartGC(time.Hour)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		w.Start(ctx)
	}()

	c := coordinator.New(database, cfg)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := c.Start(ctx); err != nil {
			log.Printf("coordinator: %v", err)
		}
	}()

	fmt.Printf("ctrq listening on :%d  (ui=%v)\n", cfg.Port, cfg.UIEnabled)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	fmt.Println("\nshutting down...")
	cancel()
	wg.Wait()
	fmt.Println("done")
}

func seedGroups(database *db.DB, cfg *models.Config) error {
	for _, gc := range cfg.Groups {
		g := &models.Group{
			Name:         gc.Name,
			PoolLimit:    gc.PoolLimit,
			AllowedTypes: gc.AllowedTypes,
		}
		if g.AllowedTypes == nil {
			g.AllowedTypes = []string{}
		}
		if err := database.UpsertGroup(g); err != nil {
			return err
		}
	}
	return nil
}
