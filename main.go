package main

import (
	"bufio"
	"context"
	"flag"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/diamondburned/autoscaler/xrandr"
	"github.com/pkg/errors"
)

var (
	configPath = "autoscaler.toml"
)

func init() {
	cfg, err := os.UserConfigDir()
	if err == nil {
		configPath = filepath.Join(cfg, "autoscaler.toml")
	}
}

func main() {
	flag.StringVar(&configPath, "c", configPath, "path to autoscaler.toml")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	cfg, err := readConfig(configPath)
	if err != nil {
		log.Fatalln("cannot read config:", err)
	}

	if err := run(ctx, cfg); err != nil && err != ctx.Err() {
		log.Fatalln(err)
	}
}

func run(ctx context.Context, cfg config) error {
	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error)
	xevEvents := make(chan string, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()

		if err := xevRoot(ctx, cfg.Events, xevEvents); err != nil {
			select {
			case <-ctx.Done():
				return
			case errCh <- err:
				cancel()
			}
		}
	}()

	do := func() error {
		screens, err := xrandr.Query(ctx)
		if err != nil {
			return errors.Wrap(err, "xrandr")
		}

		screen, ok := screens.Find(cfg.Screen)
		if !ok {
			log.Println("missing screen", cfg.Screen)
			return nil
		}

		w, h := screen.Resolution()

		scale, ok := cfg.findScale(w, h)
		if !ok {
			return nil
		}

		if err := cfg.runCommand(ctx, map[string]string{
			"scale":  strconv.FormatFloat(scale, 'f', -1, 64),
			"width":  strconv.Itoa(w),
			"height": strconv.Itoa(h),
		}); err != nil {
			log.Println("command error:", err)
		}

		return nil
	}

	doDebounced := func() error {
		debounce := time.NewTimer(time.Duration(cfg.Debounce))
		defer debounce.Stop()

		if err := do(); err != nil {
			return err
		}

		// TODO: maybe combine this into 1 select by setting xevEvents to nil.
		select {
		case err := <-errCh:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-debounce.C:
			return nil
		}
	}

	for {
		if err := doDebounced(); err != nil {
			return err
		}

		select {
		case err := <-errCh:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-xevEvents:
			// do
		}
	}
}

func xevRoot(ctx context.Context, filterEvents []string, eventCh chan<- string) error {
	cmd := exec.CommandContext(ctx, "xev", "-root")
	cmd.Stderr = os.Stderr

	out, err := cmd.StdoutPipe()
	if err != nil {
		return errors.Wrap(err, "xev: cannot get stdout")
	}
	defer out.Close()

	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "xev: cannot start")
	}

	scanner := bufio.NewScanner(out)
scanLoop:
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), " ", 2)
		if len(parts) != 2 {
			continue
		}

		for _, ev := range filterEvents {
			if ev == parts[0] {
				// Ensure the channel is filled.
				select {
				case eventCh <- ev:
				default:
				}
				continue scanLoop
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "xev: cannot scan stdout")
	}

	if err := cmd.Wait(); err != nil {
		return errors.Wrap(err, "xev: exited unexpectedly")
	}

	return nil
}
