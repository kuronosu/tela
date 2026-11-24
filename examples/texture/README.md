# texture

Content that the application renders itself, inside a Tela window (`tela.UseTexture` and `h.Texture`, ADR-315): an animated scene, a ringed planet with four moons, drawn by the example's own Vulkan code on the GPU that draws Tela's windows, and composed by Tela as any image: inside a rounded card that clips it, under a label and over controls (pause, speed, who draws).

It is a module of its own (`go.mod` here, with `replace github.com/kuronosu/tela => ../..`): an application with a graphics API of its own. The API is Vulkan, called through purego without cgo, which links a Linux binary to the C library, as Tela's own GPU backend does (ADR-281). Run everything for it from this folder.

## How a frame gets to the screen

| File | What it does |
|---|---|
| `stream.go` | The loop, on a `tela.UseGo` goroutine: it waits for the window's next frame (`Texture.NextFrame`), advances the scene's clock to it, draws at `Texture.Size` and submits. There is no timer: paused, or out of sight, the goroutine waits and nothing runs. `Texture.OnResize` wakes a paused loop to draw once at the new size |
| `gpu_linux.go` | The GPU painter. `Texture.Device` names the GPU that draws Tela's windows, which it finds among Vulkan's by the device UUID (else by the DRM render node), and lists the formats that GPU imports: the painter takes the opaque ones first, in a tiled layout that its Vulkan device draws into. It allocates up to five buffers (with fences Tela may hold four), imports each one once (`Texture.Import`) and submits it with `tela.SyncFile`; `Release` gives it back. A resize frees the idle buffers of the old size, and the others as Tela lets go of them |
| `vulkan/` | The renderer: images exported as dmabufs (`VK_EXT_image_drm_format_modifier`, `VK_KHR_external_memory_fd`), a semaphore exported as a sync file for each frame (`VK_KHR_external_semaphore_fd`), and the barrier that leaves the image in the general layout and gives it to the foreign queue family. Only the structs and commands that takes; `TestLayout` has what a C compiler says of the structs |
| `software.go`, `shade.go` | The same scene in Go, submitted as pixels (`Texture.SubmitImage`): what runs without a GPU (a default build, `TELA_RENDERER=cpu`), on Windows and on macOS, and when Vulkan fails, with the reason in the window |
| `scene.go`, `shaders/` | The scene: positions computed in Go, each pixel by the fragment shader |

**Another GPU.** The second choice draws on a GPU that is not Tela's, from the formats `Texture.Device` lists for foreign memory, which are linear, in system memory: no GPU is sure to reach the memory of another one (NVIDIA's cannot be read by Intel's). NVIDIA draws into no linear image, so there each frame is drawn beside the buffer and copied into it, on the GPU. Where Tela draws with the CPU this choice takes the first GPU there is, and Tela maps and copies its frames.

**Ending.** When the component unmounts, or another choice replaces the painter, the loop ends and its painter waits until Tela let go of every buffer (the shown one when another frame replaces it), then frees them and closes the device.

## Run it

- With Tela's GPU backend, no copy of a frame leaves the GPU: `CGO_ENABLED=0 go run -tags telagpu .`
- In software, the same scene: `CGO_ENABLED=0 go run .`, or the first binary with `TELA_RENDERER=cpu`.
- Without a window: `CGO_ENABLED=0 go run github.com/kuronosu/tela/cmd/tela snapshot --size 960x760 --scale 2 -o texture.png .` (a still of the scene, drawn in software).
- Tests: `CGO_ENABLED=0 go test ./...`. The goldens (`testdata/texture_x1.png`, `texture_x2.png`) and the aria snapshot need no GPU; regenerate them with `TELA_UPDATE=1`. The Vulkan tests run on every GPU of the system, skip without one, and compare what it drew, read through the mapped dmabuf or from the window, with the software renderer (`TELA_VK_VALIDATE=1` turns on the Khronos validation layer, and objects left alive fail). `go test -tags telagpu ./...` adds the frames imported by Tela's renderer on each GPU, the frames' own and another one. `go test -run xxx -bench .` measures both renderers.
- The shaders are compiled with `go generate` (glslangValidator); the `.spv` files are checked in.

At 1532x862, the size of the scene in the window at scale 2, a frame takes an RTX 3060 Ti 0.09 ms (0.6 ms when it is for another GPU), a UHD Graphics 630 1.4 ms (1.7 ms), and the software renderer 13 ms on the 12 threads of an i5-10400.
