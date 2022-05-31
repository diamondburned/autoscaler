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
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/diamondburned/autoscaler/xrandr"
	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
)

type config struct {
	Screen  string   `toml:"screen"`
	Command string   `toml:"command"`
	Events  []string `toml:"events"`
	Scales  []scale  `toml:"scale"`
}

func (cfg config) runCommand(ctx context.Context, extraEnv map[string]string) error {
	env := os.Environ()
	for k, v := range extraEnv {
		env = append(env, k+"="+v)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", cfg.Command)
	cmd.Env = env
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

// findScale finds the biggest scale.
func (cfg config) findScale(w, h int) (scale, bool) {
	for i := len(cfg.Scales) - 1; i >= 0; i-- {
		scale := cfg.Scales[i]
		if scale.Width != 0 && scale.Width > w {
			continue
		}
		if scale.Height != 0 && scale.Height > h {
			continue
		}
		return scale, true
	}
	return scale{}, false
}

type scale struct {
	Width  int     `toml:"width"`
	Height int     `toml:"height"`
	Scale  float64 `toml:"scale"`
}

func readConfig(filename string) (config, error) {
	var cfg config

	f, err := os.Open(filename)
	if err != nil {
		return cfg, errors.Wrap(err, "cannot open config file")
	}
	defer f.Close()

	if err := toml.NewDecoder(f).Decode(&cfg); err != nil {
		return cfg, errors.Wrap(err, "cannot parse TOML config")
	}

	sort.Slice(cfg.Scales, func(i, j int) bool {
		return cfg.Scales[i].Scale < cfg.Scales[j].Scale
	})

	return cfg, nil
}

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
			errCh <- err
			cancel()
		}
	}()

	for {
		screens, err := xrandr.Query(ctx)
		if err != nil {
			return errors.Wrap(err, "xrandr")
		}

		screen, ok := screens.Find(cfg.Screen)
		if !ok {
			log.Println("missing screen", cfg.Screen)
			continue
		}

		w, h := screen.Resolution()

		scale, ok := cfg.findScale(w, h)
		if !ok {
			continue
		}

		err = cfg.runCommand(ctx, map[string]string{
			"scale":  strconv.FormatFloat(scale.Scale, 'f', -1, 64),
			"width":  strconv.Itoa(w),
			"height": strconv.Itoa(h),
		})
		if err != nil {
			log.Println("command error:", err)
		}

		select {
		case err := <-errCh:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-xevEvents:
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
