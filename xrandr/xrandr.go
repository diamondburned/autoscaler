package xrandr

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"

	"github.com/diamondburned/sfmatch"
	"github.com/pkg/errors"
)

// Screen describes a current XRandR screen.
type Screen struct {
	Name      string
	Connected bool
	Dimension string
}

// Resolution returns the resolution of the screen.
func (s Screen) Resolution() (w, h int) {
	_, err := fmt.Sscanf(s.Dimension, "%dx%d", &w, &h)
	if err != nil {
		log.Panicln("BUG: Resolution:", err)
	}
	return
}

// Screens composes multiple screens.
type Screens []Screen

// Find finds the screen with the given name.
func (s Screens) Find(name string) (Screen, bool) {
	for _, screen := range s {
		if screen.Name == name {
			return screen, true
		}
	}
	return Screen{}, false
}

type screen struct {
	_         string `^`
	Name      string `([A-Za-z0-9]+?)`
	Connected string `((?:dis)?connected)`
	Dimension string `(\d+x\d+\+\d+\+\d+)`
}

var screenMatcher = sfmatch.MustCompile((*screen)(nil))

// Parse parses the output of xrandr.
func Parse(r io.Reader) (Screens, error) {
	scanner := bufio.NewScanner(r)
	scanner.Scan() // skip first line

	var screens Screens

	for scanner.Scan() {
		text := scanner.Text()
		if text == "" || strings.HasPrefix(text, "   ") {
			continue
		}

		var s screen
		if err := screenMatcher.Unmarshal(text, &s); err != nil {
			return nil, errors.Wrapf(err, "cannot parse line %q", text)
		}

		screens = append(screens, Screen{
			Name:      s.Name,
			Connected: s.Connected == "connected",
			Dimension: s.Dimension,
		})
	}

	return screens, scanner.Err()
}

// Query queries the system's xrandr output.
func Query(ctx context.Context) (Screens, error) {
	cmd := exec.CommandContext(ctx, "xrandr")

	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, "cannot get stdout")
	}
	defer out.Close()

	if err := cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "cannot start")
	}

	screens, err := Parse(out)
	if err != nil {
		return nil, errors.Wrap(err, "cannot parse")
	}

	if err := cmd.Wait(); err != nil {
		return nil, errors.Wrap(err, "exited unexpectedly")
	}

	return screens, nil
}
