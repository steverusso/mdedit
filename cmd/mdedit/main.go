package main

import (
	"errors"
	"flag"
	"fmt"
	"image/color"
	"io/fs"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/steverusso/mdedit"
	"github.com/steverusso/mdedit/fonts"
)

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

func (fs *diskFS) HomeDir() string {
	return fs.homeDir
}

func (fs *diskFS) WorkingDir() string {
	return fs.workingDir
}

func (_ *diskFS) ReadDir(fpath string) ([]fs.FileInfo, error) {
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

func (_ *diskFS) ReadFile(fpath string) ([]byte, error) {
	if fpath == "" {
		return nil, errors.New("empty file path")
	}
	return os.ReadFile(fpath)
}

func (_ *diskFS) WriteFile(fpath string, data []byte) error {
	return os.WriteFile(fpath, data, 0o644)
}

func run() error {
	win := app.NewWindow(
		app.Size(unit.Dp(1500), unit.Dp(900)),
		app.Title("MdEdit"),
	)
	win.Perform(system.ActionCenter)

	th := material.NewTheme(fonts.UbuntuFontCollection)
	th.TextSize = unit.Dp(17)
	th.Palette = material.Palette{
		Bg:         color.NRGBA{17, 21, 24, 255},
		Fg:         color.NRGBA{235, 235, 235, 255},
		ContrastFg: color.NRGBA{10, 180, 230, 255},
		ContrastBg: color.NRGBA{220, 220, 220, 255},
	}

	fs, err := newDiskFS()
	if err != nil {
		return err
	}

	s := mdedit.NewSession(fs, win)
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
			paint.Fill(gtx.Ops, th.Palette.Bg)
			s.Layout(gtx, th)
			e.Frame(gtx.Ops)
			log.Println(time.Now().Sub(start))
		case key.Event:
			if e.State != key.Press {
				break
			}
			switch e.Modifiers {
			case key.ModCtrl:
				switch e.Name {
				case "O":
					s.OpenFileExplorerTab()
				case "W":
					s.CloseActiveTab()
				case key.NameTab:
					s.NextTab()
				}
			case key.ModCtrl | key.ModShift:
				switch e.Name {
				case key.NamePageUp:
					s.SwapTabUp()
				case key.NamePageDown:
					s.SwapTabDown()
				case key.NameTab:
					s.PrevTab()
				}
			case key.ModAlt:
				if strings.Contains("123456789", e.Name) {
					if n, err := strconv.Atoi(e.Name); err == nil {
						s.SelectTab(n - 1)
					}
				}
			}
			win.Invalidate()
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
