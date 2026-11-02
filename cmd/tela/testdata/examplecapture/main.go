// Package main holds capture.go, the file TestExamplesCocoa adds to every
// example's package main (cmd/tela/examples_cocoa_e2e_test.go). This main
// only makes the directory a program, so that
//
//	CGO_ENABLED=0 GOOS=darwin go vet ./cmd/tela/testdata/examplecapture
//
// checks capture.go on any OS (go vet ./... skips testdata). The test reads
// capture.go only.
package main

func main() {}
