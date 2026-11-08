// Package notoemoji embeds Noto Color Emoji 2.051, the COLRv1 build
// (vector, scalable and drawn in color at any size), for apps that want
// the same emoji on every machine instead of the system's. Importing it
// adds 5.0 MB to the binary, the largest of Tela's font packages; an app
// that does not import it carries no font file and draws emoji with the
// system's emoji font (Apple Color Emoji, Segoe UI Emoji, Noto Color Emoji
// where installed).
//
//	import "github.com/kuronosu/tela/fonts/notoemoji"
//
//	tela.Main(tela.App{
//	    Title: "Chat",
//	    Fonts: []tela.Font{notoemoji.Emoji},
//	    Root:  Chat(),
//	})
//
// With it, the emoji generic name draws Noto Color Emoji, which is where
// text looks for characters its own font lacks and that are emoji.
//
// Noto Color Emoji is licensed under the SIL Open Font License 1.1,
// Copyright 2022 Google Inc. (https://github.com/googlefonts/noto-emoji).
package notoemoji

import (
	"github.com/kuronosu/tela"
	asset "github.com/kuronosu/tela/internal/assets/fonts/notocoloremoji"
)

// Emoji is Noto Color Emoji as the emoji family.
var Emoji = tela.FontData(asset.Family, asset.Data).Emoji()
