# texture

Content that the application renders itself, inside a Tela window (`tela.UseTexture` and `h.Texture`, ADR-315): one of three animated scenes (a ringed planet, mountains in the mist at dusk, folds of satin), drawn by the example's own Vulkan code on the GPU that draws Tela's windows, and composed by Tela as any image: inside a rounded card that clips it, under a label and over controls (the scene, the resolution of its frames, the size of the card, pause, speed, who draws).

It is a module of its own (`go.mod` here, with `replace github.com/kuronosu/tela => ../..`): an application with a graphics API of its own. The API is Vulkan, called through purego without cgo, which links a Linux binary to the C library, as Tela's own GPU backend does (ADR-281). Run everything for it from this folder.

## How a frame gets to the screen

| File | What it does |
|---|---|
| `stream.go` | The loop, on a `tela.UseGo` goroutine: it waits for the window's next frame (`Texture.NextFrame`), advances the scene's clock to it, draws the chosen scene at the chosen share of `Texture.Size` and submits. There is no timer: paused, or out of sight, the goroutine waits and nothing runs. `Texture.OnResize` and the selectors wake a paused loop to draw once |
| `gpu_linux.go` | The GPU painter. `Texture.Device` names the GPU that draws Tela's windows, which it finds among Vulkan's by the device UUID (else by the DRM render node), and lists the formats that GPU imports: the painter takes the opaque ones first, in a tiled layout that its Vulkan device draws into. It allocates up to five buffers (with fences Tela may hold four), imports each one once (`Texture.Import`) and submits it with `tela.SyncFile`; `Release` gives it back. Another size frees the idle buffers of the old one, and the others as Tela lets go of them |
| `vulkan/` | The renderer: images exported as dmabufs (`VK_EXT_image_drm_format_modifier`, `VK_KHR_external_memory_fd`), a semaphore exported as a sync file for each frame (`VK_KHR_external_semaphore_fd`), and the barrier that leaves the image in the general layout and gives it to the foreign queue family. Only the structs and commands that takes; `TestLayout` has what a C compiler says of the structs |
| `software.go`, `shade.go`, `planet.go`, `mountains.go`, `satin.go` | The same scenes in Go, submitted as pixels (`Texture.SubmitImage`): what runs without a GPU (a default build, `TELA_RENDERER=cpu`), on Windows and on macOS, and when Vulkan fails, with the reason in the window |
| `scene.go`, `shaders/` | The scenes: positions and times computed in Go, each pixel by the fragment shader |

**Scenes.** One fragment shader draws all three, which a push constant chooses. The shader and the Go code draw the same pixels, within one level: they are made of sums, products, divisions, square roots and integer hashes, with no sine, exponential or power, which every GPU rounds in its own way. What changes slowly (the folds of the satin, the clouds) is computed every few pixels and interpolated, on both.

**Sizes.** Resolution is the size of the frames, 25% to 200% of the device pixels they are shown at (at least 16 pixels, at most what the device and the texture take); the element's size does not change, and Tela scales the texture into it. Size is the card's: the loop follows it as it follows the window, at the same share. In a short window the scene is narrower than the card, to leave room for the controls below it; what does not fit scrolls.

**Another GPU.** The second choice draws on a GPU that is not Tela's, from the formats `Texture.Device` lists for foreign memory, which are linear, in system memory: no GPU is sure to reach the memory of another one (NVIDIA's cannot be read by Intel's). NVIDIA draws into no linear image, so there each frame is drawn beside the buffer and copied into it, on the GPU. Where Tela draws with the CPU this choice takes the first GPU there is, and Tela maps and copies its frames.

**Ending.** When the component unmounts, or another choice replaces the painter, the loop ends and its painter waits until Tela let go of every buffer (the shown one when another frame replaces it), then frees them and closes the device.

## Run it

- With Tela's GPU backend, no copy of a frame leaves the GPU: `CGO_ENABLED=0 go run -tags telagpu .`
- In software, the same scenes: `CGO_ENABLED=0 go run .`, or the first binary with `TELA_RENDERER=cpu`.
- Without a window: `CGO_ENABLED=0 go run github.com/kuronosu/tela/cmd/tela snapshot --preview Planet --size 960x760 --scale 2 -o texture.png .` (a still, drawn in software; the other previews are `Mountains` and `Satin`).
- Tests: `CGO_ENABLED=0 go test ./...`. The goldens (`testdata`) and the aria snapshot need no GPU; regenerate them with `TELA_UPDATE=1`. The Vulkan tests run on every GPU of the system, skip without one, and compare what it drew, read through the mapped dmabuf or from the window, with the software renderer (`TELA_VK_VALIDATE=1` turns on the Khronos validation layer, and objects left alive fail). `go test -tags telagpu ./...` adds the frames imported by Tela's renderer on each GPU, the frames' own and another one. `go test -run xxx -bench .` measures both renderers.
- The shaders (`scene.frag` includes the others) are compiled with `go generate` (glslangValidator); the `.spv` files are checked in.

At 1532x862, the size of the scene in the window at scale 2, a frame takes (ms):

| | Planet | Mountains | Satin |
|---|---|---|---|
| RTX 3060 Ti | 0.4 | 0.4 | 0.5 |
| RTX 3060 Ti, for another GPU | 0.9 | 0.8 | 0.9 |
| UHD Graphics 630 | 10.1 | 10.2 | 14.9 |
| Software, the 12 threads of an i5-10400 | 14.5 | 14.3 | 14.2 |

A smaller resolution takes proportionally less; 200% takes four times as much.
