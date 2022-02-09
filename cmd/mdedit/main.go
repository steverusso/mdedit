package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/steverusso/mdedit"
	"github.com/steverusso/mdedit/fonts"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

func readFile(fpath string) ([]byte, error) {
	if fpath == "" {
		return nil, errors.New("empty file path")
	}
	data, err := os.ReadFile(fpath)
	if err != nil {
		return nil, fmt.Errorf("reading '%s': %w\n", fpath, err)
	}
	return data, nil
}

func readDir(fpath string) ([]fs.FileInfo, error) {
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

type session struct {
	win        *app.Window
	workingDir string
	homeDir    string
	tabs       []tab
	tabList    layout.List
	activeTab  int
}

type tab struct {
	btn     widget.Clickable
	content tabContent
}

func newSession(win *app.Window) session {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("getting home dir: %v\n", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		log.Printf("getting working dir: %v\n", err)
	}
	return session{
		win:        win,
		homeDir:    home,
		workingDir: cwd,
		tabList:    layout.List{Axis: layout.Vertical},
	}
}

func (s *session) layout(gtx C, th *material.Theme) D {
	if len(s.tabs) == 0 {
		return layout.Center.Layout(gtx, func(gtx C) D {
			return material.Body1(th, "Use Ctrl-O to open a file!").Layout(gtx)
		})
	}
	return layout.Flex{}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			gtx.Constraints.Min.X = 220
			gtx.Constraints.Max.X = 220
			return s.tabList.Layout(gtx, len(s.tabs), func(gtx C, i int) D {
				t := &s.tabs[i]
				if t.btn.Clicked() {
					s.selectTab(i)
					op.InvalidateOp{}.Add(gtx.Ops)
				}
				// If this is the active tab, emphasize the text and invert the bg & fg.
				lbl := material.Body1(th, t.content.title())
				lbl.Font.Variant = "Mono"
				bg := th.Bg
				if i == s.activeTab {
					lbl.Font.Weight = text.Bold
					lbl.Color = bg
					bg = th.ContrastBg
				}
				// Record the layout in order to get the size for filling the background.
				m := op.Record(gtx.Ops)
				dims := t.btn.Layout(gtx, func(gtx C) D {
					return layout.UniformInset(unit.Dp(5)).Layout(gtx, lbl.Layout)
				})
				call := m.Stop()
				// Fill the background and draw the tab button.
				rect := clip.Rect{Max: dims.Size}
				paint.FillShape(gtx.Ops, bg, rect.Op())
				call.Add(gtx.Ops)
				return dims
			})
		}),
		layout.Rigid(func(gtx C) D {
			size := image.Point{1, gtx.Constraints.Max.Y}
			rect := clip.Rect{Max: size}.Op()
			paint.FillShape(gtx.Ops, th.Fg, rect)
			return D{Size: size}
		}),
		layout.Flexed(1, func(gtx C) D {
			return s.layTabContent(gtx, th, s.tabs[s.activeTab].content)
		}),
	)
}

func (s *session) layTabContent(gtx C, th *material.Theme, t tabContent) D {
	switch t := t.(type) {
	case *markdownTab:
		return s.layMarkdownTab(gtx, th, t)
	case *explorerTab:
		return s.layExplorerTab(gtx, th, t)
	default:
		return D{}
	}
}

func (s *session) layMarkdownTab(gtx C, th *material.Theme, t *markdownTab) D {
	if t.view.Editor.SaveRequested() {
		go s.writeFile(t.name, t.view.Editor.Text())
	}
	return mdedit.ViewStyle{
		Theme:      th,
		EditorFont: text.Font{Variant: "Mono"},
		Palette: mdedit.Palette{
			Fg:         th.Palette.Fg,
			Bg:         th.Palette.Bg,
			LineNumber: color.NRGBA{200, 180, 4, 125},
			Heading:    color.NRGBA{200, 193, 255, 255},
			ListMarker: color.NRGBA{10, 190, 240, 255},
			BlockQuote: color.NRGBA{165, 165, 165, 230},
			CodeBlock:  color.NRGBA{162, 120, 70, 255},
		},
		View: &t.view,
	}.Layout(gtx)
}

func (s *session) layExplorerTab(gtx C, th *material.Theme, t *explorerTab) D {
	for _, e := range t.expl.Events() {
		switch e := e.(type) {
		case mdedit.ChoseDir:
			go s.openExplorerDir(t, e.Path)
		case mdedit.ChoseFiles:
			go func() {
				for i, fpath := range e.Paths {
					s.openFile(fpath)
					if i == 0 {
						s.closeActiveTab()
						s.win.Invalidate()
					}
				}
			}()
		}
	}
	return t.expl.Layout(gtx, th)
}

func (s *session) openFileExplorerTab() {
	s.tabs = append(s.tabs, tab{content: &explorerTab{
		expl: mdedit.NewExplorer(s.homeDir, s.workingDir),
	}})
	s.nextTab()
}

func (s *session) openFile(fpath string) {
	data, err := readFile(fpath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Println(err)
		return
	}
	name := fpath
	if fpath[0] == '/' {
		rel, err := filepath.Rel(s.workingDir, fpath)
		if err != nil {
			log.Printf("getting relative path '%s': %w\n", fpath, err)
			return
		}
		name = rel
	}
	md := &markdownTab{name: name}
	md.view.Editor.SetText(data)
	md.view.SplitRatio = 0.5
	s.tabs = append(s.tabs, tab{content: md})
	s.win.Invalidate()
}

func (s *session) openExplorerDir(t *explorerTab, fpath string) {
	if fpath == "" {
		return
	}
	fpath = path.Clean(fpath)
	files, err := readDir(fpath)
	if err != nil {
		log.Println(err)
		return
	}
	sort.SliceStable(files, func(i, j int) bool {
		a, b := files[i], files[j]
		if a.IsDir() == b.IsDir() {
			return a.Name() < b.Name()
		}
		return a.IsDir()
	})
	t.expl.Populate(fpath, files)
	s.win.Invalidate()
}

func (s *session) writeFile(fpath string, data []byte) {
	if err := os.WriteFile(fpath, data, 0o644); err != nil {
		log.Println(err)
	}
}

func (s *session) closeActiveTab() {
	s.tabs = append(s.tabs[:s.activeTab], s.tabs[s.activeTab+1:]...)
	n := len(s.tabs)
	if s.activeTab > 0 && s.activeTab >= n {
		s.activeTab = n - 1
	}
	if n > 0 {
		s.tabs[s.activeTab].content.focus()
	}
}

func (s *session) focusActiveTab() {
	if i := s.activeTab; i >= 0 && i < len(s.tabs) {
		s.tabs[i].content.focus()
	}
}

func (s *session) nextTab() {
	if s.activeTab < len(s.tabs)-1 {
		s.activeTab++
	}
}

func (s *session) prevTab() {
	if s.activeTab > 0 {
		s.activeTab--
	}
}

func (s *session) swapTabUp() {
	if s.activeTab == 0 {
		return
	}
	i := s.activeTab
	j := i - 1
	s.tabs[i], s.tabs[j] = s.tabs[j], s.tabs[i]
	s.activeTab--
}

func (s *session) swapTabDown() {
	if s.activeTab == len(s.tabs)-1 {
		return
	}
	i := s.activeTab
	j := i + 1
	s.tabs[i], s.tabs[j] = s.tabs[j], s.tabs[i]
	s.activeTab++
}

func (s *session) selectTab(n int) {
	if len(s.tabs) == 0 || n < 0 {
		return
	}
	if n >= len(s.tabs) {
		n = len(s.tabs) - 1
	}
	s.activeTab = n
	s.tabs[s.activeTab].content.focus()
}

type tabContent interface {
	title() string
	focus()
}

type markdownTab struct {
	name string
	view mdedit.View
}

func (t *markdownTab) title() string {
	return t.name
}

func (t *markdownTab) focus() {
	t.view.Editor.Focus()
}

type explorerTab struct {
	expl *mdedit.Explorer
}

func (t *explorerTab) title() string {
	return "[Chose Files]"
}

func (t *explorerTab) focus() {}

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

	s := newSession(win)
	for _, fpath := range flag.Args() {
		s.openFile(fpath)
	}
	s.focusActiveTab()

	var ops op.Ops
	for {
		e := <-win.Events()
		switch e := e.(type) {
		case system.FrameEvent:
			start := time.Now()
			gtx := layout.NewContext(&ops, e)
			paint.Fill(gtx.Ops, th.Palette.Bg)
			s.layout(gtx, th)
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
					s.openFileExplorerTab()
				case "W":
					s.closeActiveTab()
				case key.NameTab:
					s.nextTab()
				}
			case key.ModCtrl | key.ModShift:
				switch e.Name {
				case key.NamePageUp:
					s.swapTabUp()
				case key.NamePageDown:
					s.swapTabDown()
				case key.NameTab:
					s.prevTab()
				}
			case key.ModAlt:
				if strings.Contains("123456789", e.Name) {
					if n, err := strconv.Atoi(e.Name); err == nil {
						s.selectTab(n - 1)
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
