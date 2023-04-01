package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"log"
	"os"
	"time"

	"gioui.org/app"
	"gioui.org/font/opentype"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"
	"gioui.org/widget/material"
	"github.com/steverusso/gio-fonts/inconsolata/inconsolatabold"
	"github.com/steverusso/gio-fonts/inconsolata/inconsolataregular"
	"github.com/steverusso/gio-fonts/nunito/nunitobold"
	"github.com/steverusso/gio-fonts/nunito/nunitobolditalic"
	"github.com/steverusso/gio-fonts/nunito/nunitoitalic"
	"github.com/steverusso/gio-fonts/nunito/nunitoregular"
	"github.com/steverusso/mdedit"
)

const topLevelKeySet = "Ctrl-[O,W," + key.NameTab + "]" +
	"|Ctrl-Shift-[" + key.NamePageUp + "," + key.NamePageDown + "," + key.NameTab + "]" +
	"|Alt-[1,2,3,4,5,6,7,8,9]"

var printFrameTimes = flag.Bool("print-frame-times", false, "Print how long each frame takes.")

type diskFS struct {
	homeDir    string
	workingDir string
}

func newDiskFS() (*diskFS, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home dir: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working dir: %w", err)
	}
	return &diskFS{
		homeDir:    homeDir,
		workingDir: cwd,
	}, nil
}

func (d *diskFS) HomeDir() string {
	return d.homeDir
}

func (d *diskFS) WorkingDir() string {
	return d.workingDir
}

func (*diskFS) ReadDir(fpath string) ([]fs.FileInfo, error) {
	entries, err := os.ReadDir(fpath)
	if err != nil {
		return nil, fmt.Errorf("reading '%s': %w", fpath, err)
	}
	infos := make([]fs.FileInfo, len(entries))
	for i, en := range entries {
		info, err := en.Info()
		if err != nil {
			return nil, fmt.Errorf("getting fileinfo for '%s/%s': %w", fpath, en.Name(), err)
		}
		infos[i] = info
	}
	return infos, nil
}

func (*diskFS) ReadFile(fpath string) ([]byte, error) {
	return os.ReadFile(fpath)
}

func (*diskFS) WriteFile(fpath string, data []byte) error {
	return os.WriteFile(fpath, data, 0o644)
}

func mustFont(fnt text.Font, data []byte) text.FontFace {
	face, err := opentype.Parse(data)
	if err != nil {
		panic("failed to parse font: " + err.Error())
	}
	return text.FontFace{Font: fnt, Face: face}
}

func run() error {
	win := app.NewWindow(
		app.Size(1500, 900),
		app.Title("MdEdit"),
	)
	win.Perform(system.ActionCenter)

	th := material.NewTheme([]text.FontFace{
		// Proportionals.
		mustFont(text.Font{}, nunitoregular.TTF),
		mustFont(text.Font{Weight: text.Bold}, nunitobold.TTF),
		mustFont(text.Font{Weight: text.Bold, Style: text.Italic}, nunitobolditalic.TTF),
		mustFont(text.Font{Style: text.Italic}, nunitoitalic.TTF),
		// Monos.
		mustFont(text.Font{Variant: "Mono"}, inconsolataregular.TTF),
		mustFont(text.Font{Variant: "Mono", Weight: text.Bold}, inconsolatabold.TTF),
	})
	th.TextSize = 18
	th.Palette = material.Palette{
		Bg:         color.NRGBA{17, 21, 24, 255},
		Fg:         color.NRGBA{235, 235, 235, 255},
		ContrastFg: color.NRGBA{10, 180, 230, 255},
		ContrastBg: color.NRGBA{220, 220, 220, 255},
	}

	fsys, err := newDiskFS()
	if err != nil {
		return err
	}

	s := mdedit.NewSession(fsys, win)
	for _, fpath := range flag.Args() {
		s.OpenFile(fpath)
	}
	s.FocusActiveTab()

	var ops op.Ops
	for {
		e := <-win.Events()
		switch e := e.(type) {
		case system.FrameEvent:
			start := time.Now()
			gtx := layout.NewContext(&ops, e)
			// Process any key events since the previous frame.
			for _, ke := range gtx.Events(win) {
				if ke, ok := ke.(key.Event); ok {
					s.HandleKeyEvent(ke)
				}
			}
			// Gather key input on the entire window area.
			areaStack := clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops)
			key.InputOp{Tag: win, Keys: topLevelKeySet}.Add(gtx.Ops)
			s.Layout(gtx, th)
			areaStack.Pop()

			e.Frame(gtx.Ops)
			if *printFrameTimes {
				log.Println(time.Since(start))
			}
		case system.DestroyEvent:
			return e.Err
		}
	}
}

func main() {
	flag.Parse()

	go func() {
		if err := run(); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()

	app.Main()
}
