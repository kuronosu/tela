package tela

import (
	"io/fs"

	"github.com/kuronosu/tela/internal/core"
)

// Assets registers a file system that images are loaded from: the path
// in a class such as bg-[url(img/hero.jpg)] or mask-[url(icons/star.svg)],
// or in h.SrcPath("img/logo.svg"), names a file of fsys, relative to its
// root (a leading "/" or "./" is ignored, so url(/img/hero.jpg) is the
// same file; ".." is not allowed). Call it from an init function, so that
// the files are found the same way by [Run], tela snapshot, tela inspect
// and telatest:
//
//	//go:embed img
//	var images embed.FS
//
//	func init() { tela.Assets(images) }
//
// Assets may be called several times: the file systems are searched in
// the order they were registered and the first one that has the file
// wins. Files are decoded once, when first used, and kept for the life
// of the process: PNG, JPEG, GIF (the first frame), WebP and SVG (drawn
// as vectors), recognized by their content. A missing or undecodable file
// paints nothing and is reported once in the dev log, with the path and
// the registered file systems, like an unknown class. Remote and data:
// URLs are not supported.
func Assets(fsys fs.FS) { core.RegisterAssets(fsys) }
