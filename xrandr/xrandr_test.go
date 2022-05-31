package xrandr

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/hexops/autogold"
)

func testText(t *testing.T, name string) func() io.Reader {
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("cannot read test file %q: %v", name, err)
	}
	return func() io.Reader { return bytes.NewReader(b) }
}

func TestCRD(t *testing.T) {
	readerFunc := testText(t, "test_crd.txt")

	screens, err := Parse(readerFunc())
	if err != nil {
		t.Fatal("cannot parse:", err)
	}

	v := autogold.Want("crd", []Screen{Screen{
		Name:       "screen",
		Connected:  true,
		Resolution: "3456x2160+0+0",
	}})
	v.Equal(t, screens)
}

func TestXWayland(t *testing.T) {
	readerFunc := testText(t, "test_xwayland.txt")

	screens, err := Parse(readerFunc())
	if err != nil {
		t.Fatal("cannot parse:", err)
	}

	v := autogold.Want("xwayland", []Screen{
		Screen{
			Name:       "XWAYLAND0",
			Connected:  true,
			Resolution: "1600x900+0+1",
		},
		Screen{
			Name:       "XWAYLAND8",
			Connected:  true,
			Resolution: "1920x1080+1600+0",
		},
	})
	v.Equal(t, screens)
}
