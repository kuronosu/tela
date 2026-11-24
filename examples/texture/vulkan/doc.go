// Package vulkan draws a full-screen fragment shader into buffers that other
// programs, and other GPUs, can read: images exported as dmabufs, with a sync
// file for each frame that tells when it is drawn. It is the application's own
// use of a graphics API in examples/texture, written with as little of Vulkan
// as that takes, and knows nothing of Tela.
//
// Everything of a [Renderer] is called from one goroutine at a time.
package vulkan
