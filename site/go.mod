// This module exists only to keep site/ (the Astro website, ADR-206) and its
// node_modules out of the core module: ./... patterns skip a directory that
// has its own go.mod, and so does the module zip users download.
// There is no Go code here.
module github.com/kuronosu/tela/site

go 1.27
