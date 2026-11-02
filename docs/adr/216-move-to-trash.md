# ADR-216: Moving files to the trash

- **Date:** 2026-11-01
- **Status:** Accepted. Amends ADR-210, ADR-213 and ADR-214.

## Decision

- `Desktop.MoveToTrash(path)` takes an absolute path, blocks, and is safe from any goroutine. It **never deletes**: what the trash cannot take stays where it is and the call returns `ErrUnsupported`. A missing path wraps `fs.ErrNotExist`.
- **Linux:** the Trash portal first. Without it, the freedesktop home trash, written in Go with an O_EXCL `.trashinfo` and a `RENAME_NOREPLACE` rename. Only files on the home trash's file system go there: no copy and delete, no `.Trash-$uid`.
- **Win32:** `IFileOperation`. Its progress sink refuses an item the shell would delete for good; `SHFileOperationW` cannot refuse it.
- **Darwin:** `ErrUnsupported` until phase 14.
- **Headless:** moves files only into a `SetTrashDir` folder (telatest gives one per App).
- **Remote:** protocol 5 carries it, amended before release.
- `desktop.MoveToTrash`; telatest `Trashed`, `FailTrash`, `Unsupported(telatest.Trash)`.
- **`notes`:** locking a file-backed note detaches it and offers to trash the file.

## Rationale

A locked note must not leave its text in a plain file. The owner wants nothing removed without consent and nothing deleted for good.
