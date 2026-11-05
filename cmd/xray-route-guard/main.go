package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Evangelion91/xray-route-guard/internal/guard"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "xray-route-guard:", err)
		os.Exit(1)
	}
}
func run() error {
	flags := flag.NewFlagSet("xray-route-guard", flag.ContinueOnError)
	path := flags.String("config", "/etc/xray-route-guard/config.json", "private guard configuration")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	action := "check"
	if flags.NArg() > 0 {
		action = flags.Arg(0)
	}
	if action == "version" {
		fmt.Println(version)
		return nil
	}
	if action != "check" && action != "run" && action != "status" && action != "refresh" {
		return fmt.Errorf("command must be check, run, status, refresh or version")
	}
	c, err := guard.LoadConfig(*path)
	if err != nil {
		return err
	}
	b := &guard.Backend{Config: c}
	if err = b.Prepare(); err != nil {
		return err
	}
	if action == "status" {
		data, err := os.ReadFile(filepath.Join(c.RuntimeDir, "status.json"))
		if err != nil {
			return fmt.Errorf("no status recorded")
		}
		fmt.Print(string(data))
		return nil
	}
	lock, err := guard.Lock(c.ActiveConfig + ".guard.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if action == "check" {
		if err = b.Verify(ctx); err != nil {
			return err
		}
		if !b.Health(ctx) {
			return fmt.Errorf("configuration valid, but active SOCKS health check failed")
		}
		fmt.Println("Configuration valid; active route healthy")
		return nil
	}
	if action == "refresh" {
		nodes, err := b.Refresh(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("Cached %d eligible nodes\n", len(nodes))
		return nil
	}
	if err = b.Recover(ctx); err != nil {
		return err
	}
	if err = b.Verify(ctx); err != nil {
		return err
	}
	e := &guard.Engine{Driver: b, Config: c, Nodes: b.Cached()}
	e.Status.Selected = b.CurrentID()
	for {
		err = e.Tick(ctx, time.Now().UTC())
		data, _ := json.Marshal(e.Status)
		fmt.Println(string(data))
		// Status is operational metadata only: no endpoints, labels or credentials.
		if saveErr := guard.SaveStatus(c.RuntimeDir, e.Status); saveErr != nil {
			return saveErr
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Duration(c.Interval) * time.Second):
		}
	}
}
