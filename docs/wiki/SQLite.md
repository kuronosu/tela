# SQLite

An app that keeps more than a settings file wants a database. Tela has no
database package: [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)
is SQLite translated to Go, so `CGO_ENABLED=0` still holds, and `database/sql`
is the API. This page is the recipe
[`examples/notes`](https://github.com/kuronosu/tela/tree/main/examples/notes)
uses (`store.go`, `library.go`); the example is a module of its own, so the
core module does not depend on SQLite and an app that does not need it never
pays for it.

- [Add it](#add-it)
- [Where the file goes](#where-the-file-goes)
- [Open it](#open-it)
- [Migrations](#migrations)
- [Keep it off the UI thread](#keep-it-off-the-ui-thread)
- [Test it](#test-it)

## Add it

```sh
go get modernc.org/sqlite
```

```go
import (
    "database/sql"

    _ "modernc.org/sqlite" // registers the driver "sqlite"
)
```

## Where the file goes

`desktop.DataDir()` is the app's folder for what the user would miss, per user
and per app ID: `~/.local/share/<app id>` on Linux, `%AppData%\<app id>` on
Windows and `~/Library/Application Support/<app id>` on macOS. It creates the
folder. Put the file in it, and set the app ID first (`tela.App.AppID` does
in a running app, `desktop.SetAppID` before).

```go
dir, err := desktop.DataDir()
path := filepath.Join(dir, "app.db")
```

## Open it

Options go in the DSN as `_pragma`, and run for every connection of the pool:

```go
db, err := sql.Open("sqlite", path+
    "?_pragma=journal_mode(WAL)"+     // readers never wait for the writer
    "&_pragma=busy_timeout(5000)"+    // wait for a lock instead of failing
    "&_pragma=synchronous(NORMAL)"+   // safe with WAL against a crash of the program
    "&_pragma=secure_delete(1)")      // zero what a delete leaves in a page
```

WAL (write-ahead log) turns a write into appending to `app.db-wal`: it is what
makes saving as the user types cheap. The `-wal` and `-shm` files belong to the
database; copy all of them, or close the database first (a clean close folds
the log into the main file). `db.Close()` when the app quits: `tela.Main`
returns when the last window closes, so call it after.

## Migrations

`PRAGMA user_version` holds a number in the file. Keep the schema as a list
of steps, never editing one that has shipped, and run the missing ones, each
in a transaction, when the database opens:

```go
var migrations = []string{
    `CREATE TABLE notes (id INTEGER PRIMARY KEY, title TEXT NOT NULL, body TEXT NOT NULL)`,
    `ALTER TABLE notes ADD COLUMN folder TEXT NOT NULL DEFAULT ''`,
}

func migrate(db *sql.DB) error {
    var v int
    if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
        return err
    }
    if v > len(migrations) {
        return errors.New("the file was made by a newer version of the app")
    }
    for ; v < len(migrations); v++ {
        tx, err := db.Begin()
        if err != nil {
            return err
        }
        if _, err := tx.Exec(migrations[v]); err != nil {
            tx.Rollback()
            return err
        }
        tx.Exec("PRAGMA user_version = " + strconv.Itoa(v+1)) // PRAGMA takes no parameters
        if err := tx.Commit(); err != nil {
            return err
        }
    }
    return nil
}
```

A file that is not a database (a `file is not a database` error at the first
query) is the user's: rename it to `app.db.corrupt` and start a new one
instead of deleting it. Data from an older format (a JSON file) is read once,
inside the first migration's transaction or right after it, and set aside.

## Keep it off the UI thread

A query takes milliseconds, a first open (migrations, a big file) more, and a
slow disk much more. Neither belongs in a build or a handler:

- **Read** with [`tela.UseAsync`](API-Reference-tela#func-useasync): the
  component shows a loading state, then `Data`.
- **Write** from a goroutine that keeps only the latest state: the handler
  hands over a snapshot and returns, and the goroutine writes what changed in
  one transaction. A burst of keystrokes becomes one write, and a locked
  database never blocks a frame.

```go
func (s *store) Put(snap snapshot) { // called from handlers and effects
    s.mu.Lock()
    defer s.mu.Unlock()
    s.queued = &snap // replaces one that was not written yet
    if !s.writing {
        s.writing = true
        go s.drain() // writes until nothing is queued
    }
}
```

```go
lib := tela.UseAsync(func(ctx context.Context) (library, error) {
    return openAndLoad() // open, migrate, SELECT everything the first screen needs
})
if lib.Data.store == nil {
    return P(Text("Opening your notes…"))
}
```

`examples/notes` also gives a first run, which has nothing to read, its first
screen at once, and opens the file on the writer's goroutine. Give the writer
a way to report an error (a toast through `tela.Post`) and to `Flush` before
the app quits.

## Test it

`telatest.TempDesktopDirs(t)` points `desktop.DataDir` at a temporary folder
per test. A test that wants to see what was written waits for the writer
(`Flush`), then opens the database again with its own handle, which WAL allows
beside the app's. `:memory:` with `db.SetMaxOpenConns(1)` is a database for
`tela snapshot`.

## Not covered

An ORM or a query builder (use what you like with `database/sql`), encryption
of the file (SQLite has none built in; `examples/notes` encrypts the notes
that the user locks before they reach it), and sync, which is the example's
`sync.go`.
