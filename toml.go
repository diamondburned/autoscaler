package main

import (
	"context"
	"os"
	"os/exec"
	"sort"
	"strconv"

	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
)

type number float64

func (n *number) UnmarshalText(t []byte) error {
	f, err := strconv.ParseFloat(string(t), 64)
	if err != nil {
		return err
	}
	*n = number(f)
	return nil
}

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
func (cfg config) findScale(w, h int) (float64, bool) {
	for i := len(cfg.Scales) - 1; i >= 0; i-- {
		scale := cfg.Scales[i]
		if scale.Width != 0 && scale.Width > w {
			continue
		}
		if scale.Height != 0 && scale.Height > h {
			continue
		}
		return float64(scale.Scale), true
	}
	return -1, false
}

type scale struct {
	Width  int    `toml:"width"`
	Height int    `toml:"height"`
	Scale  number `toml:"scale"`
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
