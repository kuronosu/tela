package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// The types below mirror the Wayland protocol XML format (see wayland.dtd).

type protocol struct {
	Name        string       `xml:"name,attr"`
	Copyright   string       `xml:"copyright"`
	Description *description `xml:"description"`
	Interfaces  []*iface     `xml:"interface"`

	// File is the base name of the XML file, for the generated header.
	File string `xml:"-"`
}

type description struct {
	Summary string `xml:"summary,attr"`
	Text    string `xml:",chardata"`
}

type iface struct {
	Name        string       `xml:"name,attr"`
	Version     int          `xml:"version,attr"`
	Description *description `xml:"description"`
	Requests    []*message   `xml:"request"`
	Events      []*message   `xml:"event"`
	Enums       []*enum      `xml:"enum"`
}

type message struct {
	Name            string       `xml:"name,attr"`
	Type            string       `xml:"type,attr"`
	Since           int          `xml:"since,attr"`
	DeprecatedSince int          `xml:"deprecated-since,attr"`
	Description     *description `xml:"description"`
	Args            []*arg       `xml:"arg"`
}

func (m *message) since() int {
	if m.Since == 0 {
		return 1
	}
	return m.Since
}

type arg struct {
	Name      string `xml:"name,attr"`
	Type      string `xml:"type,attr"`
	Interface string `xml:"interface,attr"`
	AllowNull string `xml:"allow-null,attr"`
	Enum      string `xml:"enum,attr"`
	Summary   string `xml:"summary,attr"`
}

func (a *arg) nullable() bool { return a.AllowNull == "true" }

type enum struct {
	Name        string       `xml:"name,attr"`
	Since       int          `xml:"since,attr"`
	Bitfield    string       `xml:"bitfield,attr"`
	Description *description `xml:"description"`
	Entries     []*entry     `xml:"entry"`
}

func (e *enum) bitfield() bool { return e.Bitfield == "true" }

type entry struct {
	Name            string `xml:"name,attr"`
	Value           string `xml:"value,attr"`
	Summary         string `xml:"summary,attr"`
	Since           int    `xml:"since,attr"`
	DeprecatedSince int    `xml:"deprecated-since,attr"`
}

// parseFile reads one protocol XML file.
func parseFile(path string) (*protocol, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p protocol
	if err := xml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("%s: <protocol> has no name attribute", path)
	}
	p.File = filepath.Base(path)
	for _, i := range p.Interfaces {
		if i.Version == 0 {
			i.Version = 1
		}
		for _, e := range i.Enums {
			for _, en := range e.Entries {
				if _, err := strconv.ParseUint(en.Value, 0, 32); err != nil {
					return nil, fmt.Errorf("%s: enum %s.%s entry %q: value %q is not an unsigned 32-bit number",
						path, i.Name, e.Name, en.Name, en.Value)
				}
			}
		}
	}
	return &p, nil
}

// loadProtocols parses every path; a directory contributes its *.xml files in
// name order. The result is sorted by file name so output does not depend on
// argument order.
func loadProtocols(paths []string) ([]*protocol, error) {
	var files []string
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			files = append(files, p)
			continue
		}
		matches, err := filepath.Glob(filepath.Join(p, "*.xml"))
		if err != nil {
			return nil, err
		}
		files = append(files, matches...)
	}
	sort.Slice(files, func(i, j int) bool { return filepath.Base(files[i]) < filepath.Base(files[j]) })
	var out []*protocol
	for _, f := range files {
		p, err := parseFile(f)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no protocol XML files found in %s", strings.Join(paths, ", "))
	}
	return out, nil
}
