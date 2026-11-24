//go:build telagpu && !linux

package core

// Dmabufs and render nodes exist on Linux only.

func keepDmabuf(Dmabuf) (kept Dmabuf, ok bool) { return Dmabuf{}, false }

func closeDmabuf(Dmabuf) {}

func renderNode(uint32, uint32) string { return "" }
