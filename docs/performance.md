# Processing performance

ColorNinja automatically adjusts image-processing concurrency to the CPU
capacity available to Go. No project setting or image-quality change is needed.

- One available logical CPU: one processing worker.
- Two or four available logical CPUs: up to two or four workers respectively.
- Eight available logical CPUs: up to seven workers.
- Larger CPUs: up to 16 workers, when the workload and working memory allow it.

On larger CPUs, leaving capacity available helps the desktop remain responsive.
Small CPUs retain their available concurrency so smoothing and mapping do not
slow down from reserving an entire core. Small jobs
use fewer workers because scheduling them can cost more than it saves. Deep
stacks and wide search beams also reduce concurrency to limit extra working
memory. The engine uses a 32 MiB budget for its **estimate of worker scratch**;
this is not a limit on total application RAM. Source images, output images,
cached stages, and ordinary single-worker search storage still need memory.

Parallel stages include edge-preserving smoothing, palette-cluster assignment,
Color Match candidate expansion and complete-stack refinement, independent
layer-thinning trials, and mapping pixels to output colors. Other stages remain
sequential, so a larger CPU will not accelerate every operation equally.

Only one preview job processes an image at a time. Changing settings cancels the
obsolete job, waits for its workers to finish, and lets the current job reuse
the working buffers. Existing smoothing, palette-analysis, and rendered-result
caches continue to avoid unnecessary work.

Parallel processing preserves the same matching calculations, search budgets,
constraints, and deterministic tie-breaks. Floating-point weight and centroid
sums retain their original order. CPU capacity does not change the intended
palette, pixels, or print-layer plan.

For an additional manual CPU restriction, advanced users can set the standard
Go `GOMAXPROCS` environment variable before launching ColorNinja. For example,
in PowerShell:

```powershell
$env:GOMAXPROCS = '1'
.\ColorNinja-Studio.exe
```

This example makes the adaptive engine use one processing worker. Close and
restart the application after changing the environment variable.

## Measured behavior

Before/after measurements on September 10, 2026 used a Ryzen 9 7950X, Windows
amd64, and separate benchmark processes with the indicated `GOMAXPROCS` limit.
These are CPU-limited tests on this workstation, not measurements on older
physical hardware. Values are medians of three runs, with a 300 ms benchmark
target per run.

The existing `BenchmarkLayerPlanning/material-false` fixture searches four
filaments from a synthetic 45-filament library at 1.28 mm maximum depth:

| Available logical CPUs | Before | After | Speedup |
| --- | ---: | ---: | ---: |
| 1 | 123 ms | 128 ms | approximately unchanged |
| 2 | 95 ms | 55 ms | 1.7x |
| 4 | 99 ms | 37 ms | 2.7x |
| 8 | 106 ms | 31 ms | 3.4x |
| 32 | 110 ms | 32 ms | 3.4x |

With material optimization enabled, the same benchmark at eight available
CPUs improved from 237 ms to 70 ms. The existing 1200 x 900 fresh-reducer
benchmark improved from 175 ms to 118 ms at 32 CPUs; one-, two-, and four-CPU
reducer times were preserved or slightly improved. At eight CPUs it measured
173 ms before and 177 ms after, reflecting the reserved CPU capacity and
workload-dependent overhead. Stack scaling levels off when the memory budget
or remaining serial work limits additional workers.

Extra concurrency has a memory cost. In the stack fixture, cumulative
allocations increased from about 39 MB to 64 MB per operation at 32 CPUs.
These totals include temporary allocations that are collected during processing;
they are not peak resident RAM. Separate runs under Go's 64 MiB soft memory
limit completed at one, four, and 32 CPUs. Sampled process peak working sets
were approximately 19, 21, and 32 MiB after the change. This small fixture does
not establish a memory limit for large source images.

Regression checks compare complete results across CPU limits 1, 2, 4, 8, and
32, including pixels, alpha, layer maps, palette/quality reports, geometry, and
cached results. Seven workflow fixtures also matched the outputs captured
before parallelization exactly. Additional checks cover weighted-cluster
arithmetic, memory/worker limits, errors, cancellation, joining active workers,
and serialized progress reporting. The complete engine and studio suites also
pass Go's race detector.

To reproduce a benchmark, set `GOMAXPROCS` **before starting the test process**:

```powershell
. .\scripts\env.ps1
$env:GOMAXPROCS = '4'
go test ./internal/engine -run '^$' -bench BenchmarkLayerPlanning -benchtime=300ms -count=3 -benchmem
```
