package scheduler

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/robfig/cron/v3"

	"github.com/YOUR_USERNAME/nimbus-sync/internal/config"
	"github.com/YOUR_USERNAME/nimbus-sync/internal/report"
	"github.com/YOUR_USERNAME/nimbus-sync/internal/sync"
)

// Run starts a foreground daemon that syncs on cfg.Schedule (cron syntax).
// It also performs one sync immediately on startup.
func Run(cfg *config.Config) error {
	log.Printf("nimbus daemon starting, schedule: %q", cfg.Schedule)

	job := func() {
		engine := sync.NewEngine(cfg, false)
		rep, err := engine.Run("")
		if err != nil {
			log.Printf("scheduled sync error: %v", err)
		}
		if rep != nil {
			rep.PrintSummary(os.Stdout)
			if err := report.Save(cfg, rep); err != nil {
				log.Printf("saving report: %v", err)
			}
		}
	}

	// Initial run so a freshly started daemon is immediately useful.
	job()

	c := cron.New()
	if _, err := c.AddFunc(cfg.Schedule, job); err != nil {
		return err
	}
	c.Start()
	defer c.Stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	log.Printf("received %s, shutting down", s)
	return nil
}
