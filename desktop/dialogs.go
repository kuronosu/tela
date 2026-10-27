package desktop

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/platform"
)

// FileFilter is one entry of a file dialog's list of types: a label and
// the glob patterns of the file names it shows.
type FileFilter struct {
	// Name is the label shown for the entry ("Images").
	Name string
	// Patterns are glob patterns of file names: "*.png", "*.jpg", "*".
	Patterns []string
}

// DialogOption configures a file dialog: [Title], [Directory],
// [DefaultName], [Filter] and [Parent].
type DialogOption struct{ apply func(*dialogConfig) }

type dialogConfig struct {
	title, dir, name string
	filters          []FileFilter
	parent           tela.WindowHandle
}

// Title sets the dialog's title (default: the OS's, such as "Open").
func Title(title string) DialogOption {
	return DialogOption{func(c *dialogConfig) { c.title = title }}
}

// Directory sets the folder the dialog starts in (default: the OS's
// choice, usually the folder used last).
func Directory(dir string) DialogOption {
	return DialogOption{func(c *dialogConfig) { c.dir = dir }}
}

// DefaultName suggests the file name of a [SaveFile] dialog
// ("Untitled.md"). The other dialogs ignore it.
func DefaultName(name string) DialogOption {
	return DialogOption{func(c *dialogConfig) { c.name = name }}
}

// Filter adds an entry to the dialog's list of file types: a label and
// glob patterns ("*.png"). Repeat it for several entries; the first is
// selected. Without filters the dialog shows every file. [PickFolder]
// ignores it.
//
//	desktop.OpenFile(done,
//	    desktop.Filter("Images", "*.png", "*.jpg", "*.webp"),
//	    desktop.Filter("All files", "*"))
func Filter(name string, patterns ...string) DialogOption {
	f := FileFilter{Name: name, Patterns: slices.Clone(patterns)}
	return DialogOption{func(c *dialogConfig) { c.filters = append(c.filters, f) }}
}

// Filters adds several entries at once, as [Filter] does for one.
func Filters(filters ...FileFilter) DialogOption {
	fs := slices.Clone(filters)
	return DialogOption{func(c *dialogConfig) { c.filters = append(c.filters, fs...) }}
}

// Parent makes the dialog modal to w's window (tela.UseWindow). The
// default is the window the call comes from: the window whose handler or
// effect is running, else the focused window, else the main window.
func Parent(w tela.WindowHandle) DialogOption {
	return DialogOption{func(c *dialogConfig) { c.parent = w }}
}

// dialogOptions applies opts and checks the filters, panicking with the
// fix (programmer errors, ARCHITECTURE section 19).
func dialogOptions(what string, opts []DialogOption) dialogConfig {
	var c dialogConfig
	for _, o := range opts {
		if o.apply == nil {
			panic(fmt.Sprintf("desktop.%s: a zero DialogOption; build options with desktop.Title, Directory, DefaultName, Filter or Parent", what))
		}
		o.apply(&c)
	}
	for _, f := range c.filters {
		if len(f.Patterns) == 0 {
			panic(fmt.Sprintf("desktop.%s: Filter(%q) has no patterns; give at least one, such as desktop.Filter(%q, \"*.txt\") or \"*\" for every file", what, f.Name, f.Name))
		}
		for _, p := range f.Patterns {
			if p == "" || strings.ContainsAny(p, ";\x00") {
				panic(fmt.Sprintf("desktop.%s: Filter(%q) has the pattern %q; a pattern is one glob such as \"*.png\" (give several patterns as separate arguments, not joined with ';')", what, f.Name, p))
			}
		}
	}
	return c
}

// platformOptions returns the platform's options for c, parented to the
// window the request comes from.
func (c dialogConfig) platformOptions(parent platform.Window) platform.FileDialogOptions {
	o := platform.FileDialogOptions{Parent: parent, Title: c.title, Directory: c.dir, Name: c.name}
	for _, f := range c.filters {
		o.Filters = append(o.Filters, platform.FileFilter{Name: f.Name, Patterns: slices.Clone(f.Patterns)})
	}
	return o
}

// OpenFile shows the OS's open-file dialog and calls done with the chosen
// file. A path of "" with a nil error means the user canceled:
//
//	desktop.OpenFile(func(path string, err error) {
//	    switch {
//	    case err != nil:
//	        setError(err.Error())
//	    case path == "":
//	        // canceled
//	    default:
//	        load(path)
//	    }
//	}, desktop.Title("Open a note"), desktop.Filter("Notes", "*.md", "*.txt"))
//
// Call it on the UI thread: an event handler or an effect (from a
// goroutine, wrap it in tela.Post). It returns at once; done runs later on
// the UI thread, exactly once, so it may set state. The returned error
// says the dialog could not be shown, and done is then never called:
// no running app, or a desktop without file dialogs (errors.Is(err,
// [ErrUnsupported]): on Linux, no XDG Desktop Portal). Tests answer the
// dialog with telatest's App.AnswerFileDialog.
func OpenFile(done func(path string, err error), opts ...DialogOption) error {
	if done == nil {
		panic("desktop.OpenFile(nil): pass the function that receives the chosen path, such as desktop.OpenFile(func(path string, err error) { ... })")
	}
	return openFiles("OpenFile", false, func(paths []string, err error) {
		if len(paths) > 0 {
			done(paths[0], err)
			return
		}
		done("", err)
	}, opts)
}

// OpenFiles is [OpenFile] for several files: done receives the chosen
// paths, nil when the user canceled.
func OpenFiles(done func(paths []string, err error), opts ...DialogOption) error {
	if done == nil {
		panic("desktop.OpenFiles(nil): pass the function that receives the chosen paths, such as desktop.OpenFiles(func(paths []string, err error) { ... })")
	}
	return openFiles("OpenFiles", true, done, opts)
}

func openFiles(what string, multiple bool, done func([]string, error), opts []DialogOption) error {
	c := dialogOptions(what, opts)
	d, err := running(what)
	if err != nil {
		return err
	}
	o := c.platformOptions(d.PlatformWindow(window(d, c.parent)))
	o.Multiple = multiple
	err = d.Services().OpenFiles(o, func(paths []string, err error) {
		done(paths, wrap(what, err))
	})
	return wrap(what, err)
}

// SaveFile shows the OS's save-file dialog and calls done with the chosen
// path ("" with a nil error: canceled). The OS asks before overwriting an
// existing file; done only gets the path, and the app writes the file.
// [DefaultName] suggests a name. Same rules as [OpenFile].
//
//	desktop.SaveFile(func(path string, err error) {
//	    if err == nil && path != "" {
//	        save(path)
//	    }
//	}, desktop.DefaultName("Untitled.md"), desktop.Filter("Markdown", "*.md"))
func SaveFile(done func(path string, err error), opts ...DialogOption) error {
	if done == nil {
		panic("desktop.SaveFile(nil): pass the function that receives the chosen path, such as desktop.SaveFile(func(path string, err error) { ... })")
	}
	c := dialogOptions("SaveFile", opts)
	d, err := running("SaveFile")
	if err != nil {
		return err
	}
	err = d.Services().SaveFile(c.platformOptions(d.PlatformWindow(window(d, c.parent))), func(path string, err error) {
		done(path, wrap("SaveFile", err))
	})
	return wrap("SaveFile", err)
}

// PickFolder shows the OS's folder chooser and calls done with the chosen
// folder ("" with a nil error: canceled). Same rules as [OpenFile];
// filters do not apply.
func PickFolder(done func(path string, err error), opts ...DialogOption) error {
	if done == nil {
		panic("desktop.PickFolder(nil): pass the function that receives the chosen folder, such as desktop.PickFolder(func(path string, err error) { ... })")
	}
	c := dialogOptions("PickFolder", opts)
	c.filters = nil
	d, err := running("PickFolder")
	if err != nil {
		return err
	}
	err = d.Services().PickFolder(c.platformOptions(d.PlatformWindow(window(d, c.parent))), func(path string, err error) {
		done(path, wrap("PickFolder", err))
	})
	return wrap("PickFolder", err)
}
