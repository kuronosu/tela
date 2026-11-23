package lint

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Every change that breaks the public API ships a rewrite for `tela lint
// -fix` (ADR-305). The Breaking entries of the Unreleased section of
// CHANGELOG.md say which one, with a marker at the end:
//
//	- `telatest.App.Move` is renamed `App.MoveWindow`. `migrate: telatest-app-move`
//
// names the entries of Migrations (several are separated by commas), and
//
//	- `i18n` rounds a float from its shortest digits. `migrate: none -- a change of behavior`
//
// says that nothing can be rewritten, and why. An entry without a marker, a
// marker with an unknown name and "none" without a reason fail this test, so
// that a break cannot be released without having thought about it.

var (
	migrateMarker = regexp.MustCompile(`migrate:\s*([a-z0-9][a-z0-9-]*(?:\s*,\s*[a-z0-9][a-z0-9-]*)*)`)
	migrateNone   = regexp.MustCompile(`migrate:\s*none\s*--\s*(\S.*)`)

	migrateNoneStart = regexp.MustCompile(`migrate:\s*none\b`)
)

// breakingEntries returns the entries (a bullet and its continuation lines,
// joined) of the Breaking section of the Unreleased part of a changelog.
func breakingEntries(changelog string) []string {
	var entries []string
	in, unreleased, comment := false, false, false
	for _, line := range strings.Split(strings.ReplaceAll(changelog, "\r\n", "\n"), "\n") {
		switch {
		case comment:
			comment = !strings.Contains(line, "-->")
			continue
		case strings.HasPrefix(line, "<!--"):
			comment = !strings.Contains(line, "-->")
			continue
		case strings.HasPrefix(line, "## "):
			unreleased = strings.HasPrefix(line, "## [Unreleased]")
			in = false
			continue
		case !unreleased:
			continue
		case strings.HasPrefix(line, "### "):
			in = strings.TrimSpace(strings.TrimPrefix(line, "### ")) == "Breaking"
			continue
		case !in:
			continue
		case strings.HasPrefix(line, "- "):
			entries = append(entries, strings.TrimPrefix(line, "- "))
		case strings.TrimSpace(line) != "" && len(entries) > 0:
			entries[len(entries)-1] += " " + strings.TrimSpace(line)
		}
	}
	return entries
}

// breakingProblems returns what is wrong with the markers of the Breaking
// entries of a changelog, given the names of the migrations that exist.
func breakingProblems(changelog string, migrations map[string]bool) []string {
	var out []string
	for _, e := range breakingEntries(changelog) {
		text := strings.ReplaceAll(e, "`", "")
		short := e
		if len(short) > 70 {
			short = short[:70] + "..."
		}
		if migrateNoneStart.MatchString(text) {
			m := migrateNone.FindStringSubmatch(text)
			if m == nil || len(strings.Fields(m[1])) < 2 {
				out = append(out, "the Breaking entry \""+short+"\" says migrate: none without a reason (migrate: none -- why nothing can be rewritten)")
			}
			continue
		}
		m := migrateMarker.FindStringSubmatch(text)
		if m == nil {
			out = append(out, "the Breaking entry \""+short+"\" has no marker: end it with `migrate: <name>` of the rewrite that `tela lint -fix` applies (internal/lint/migrations.go), or `migrate: none -- <why>`")
			continue
		}
		for _, id := range strings.Split(m[1], ",") {
			if id = strings.TrimSpace(id); !migrations[id] {
				out = append(out, "the Breaking entry \""+short+"\" names the migration "+strconv.Quote(id)+", which is not in Migrations")
			}
		}
	}
	return out
}

func migrationIDs() map[string]bool {
	ids := map[string]bool{}
	for _, m := range Migrations {
		ids[m.ID] = true
	}
	return ids
}

func TestBreakingProblems(t *testing.T) {
	const log = `# Changelog

## [Unreleased]

<!-- a comment about the markers,
- that spans lines -->

### Breaking

- ` + "`telatest.App.Move`" + ` is renamed. ` + "`migrate: telatest-app-move`" + `
- The rounding of floats changes, and this entry
  goes on over two lines. ` + "`migrate: none -- a change of behavior`" + `
- A removed option has no marker.
- A second one has a wrong name. ` + "`migrate: nope`" + `
- Without a reason. ` + "`migrate: none`" + `
- Two at once. ` + "`migrate: telatest-app-move, desktop-launch-forwarded`" + `

### Added

- Not a break and without a marker.

## [0.1.0] - 2026-01-01

### Breaking

- An old release is not checked.
`
	got := breakingProblems(log, migrationIDs())
	want := []string{
		`"A removed option has no marker."`,
		`"nope"`,
		`says migrate: none without a reason`,
	}
	if len(got) != len(want) {
		t.Fatalf("problems = %q, want %d of them", got, len(want))
	}
	for i, w := range want {
		if !strings.Contains(got[i], strings.Trim(w, `"`)) {
			t.Errorf("problem %d = %q, want it to contain %q", i, got[i], w)
		}
	}
	if es := breakingEntries(log); len(es) != 6 {
		t.Errorf("%d entries, want 6: %q", len(es), es)
	}
	if got := breakingProblems("## [Unreleased]\n\n### Added\n\n- x\n", nil); len(got) != 0 {
		t.Errorf("a changelog without breaking entries has problems: %q", got)
	}
}

// The repository's own changelog: nothing it is about to release breaks the
// API without a rewrite or a reason.
func TestChangelogBreakingEntriesHaveMigrations(t *testing.T) {
	data, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range breakingProblems(string(data), migrationIDs()) {
		t.Error(p)
	}
}

// A migration says the release that broke the API: one of the changelog's, or
// a later one (the release that is being prepared).
func TestMigrationsNameAReleaseOfTheChangelog(t *testing.T) {
	data, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	heading := regexp.MustCompile(`(?m)^## \[(\d+)\.(\d+)\.(\d+)\]`)
	versions := map[string][3]int{}
	var newest [3]int
	for _, m := range heading.FindAllStringSubmatch(string(data), -1) {
		v := [3]int{}
		for i := range v {
			v[i], _ = strconv.Atoi(m[i+1])
		}
		versions["v"+m[1]+"."+m[2]+"."+m[3]] = v
		if less(newest, v) {
			newest = v
		}
	}
	semver := regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)
	for _, m := range Migrations {
		if _, ok := versions[m.Since]; ok {
			continue
		}
		sm := semver.FindStringSubmatch(m.Since)
		if sm == nil {
			t.Errorf("migration %s: Since %q is not a version", m.ID, m.Since)
			continue
		}
		v := [3]int{}
		for i := range v {
			v[i], _ = strconv.Atoi(sm[i+1])
		}
		if !less(newest, v) {
			t.Errorf("migration %s: Since %s is not a release of CHANGELOG.md, and is not after the newest one", m.ID, m.Since)
		}
	}
}

func less(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
