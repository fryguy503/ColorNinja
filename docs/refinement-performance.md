# Filament refinement performance

Measured locally on September 10, 2026, using the Ewoks source image (1044 ×
1507), 45 eligible filaments, and the saved desktop settings: eight distinct
filaments, up to ten runs, Deeper refinement, 32 analysis colors, a 64-color
output ceiling, 3.2 mm maximum depth, show-through reduction, and custom color
order. Automatic depth and material optimization were off.

## Measurements

Windows amd64, AMD Ryzen 9 7950X, Go 1.27.1. These are local measurements for
this configuration, not a general speed guarantee.

| Measurement | Original | Optimized |
| --- | ---: | ---: |
| Full Ewoks processing | Still refining at the 600-second test deadline | 51.93 seconds |
| Longest refinement stretch | 572.26 seconds | 39.48 seconds |
| Repeated boundary scoring benchmark, median | 138.71 microseconds | 8.01 microseconds |

The full processing measurement establishes a **lower bound of 11.5×**, not an
exact completed before/after ratio. The completed long refinement stretch was
14.5× faster. A second optimized build took 51.57 seconds and produced the same
entire serialized result as the final build.

The initial 30-second CPU profile attributed 51.9% of sampled CPU time to
`stackSurface`, including repeatedly converting endpoint colors and scoring
the same physical layer intervals during palette and height trials.

## Changes

- Cache unweighted Oklab colors and boundary interval calculations within one
  candidate's palette selection. Create the cache only after color guards pass.
- Use a direct interval table for stacks of at most 64 sampled layers. Larger
  stacks use a sparse cache capped at 4096 entries; cache misses beyond that cap
  still compute the complete original score.
- Precompute the exact sRGB transfer function for its 256 possible byte inputs,
  replacing repeated power calculations with a 2 KiB lookup table.

Each palette selection owns its cache. Nothing is shared between workers or
retained across candidate stacks. Segment endpoint order, layer traversal, and
weighted accumulation order are preserved to avoid rounding changes near search
ties. Search budgets, candidate order, optical blending, color tolerances,
filament constraints, and the output palette objective are unchanged.

## Validation and limits

- `go test ./...` and `go vet ./...` passed.
- Exact comparisons against the frozen original boundary scorer cover changing
  palettes, repeated RGBs at different heights, both color pipelines, varied
  boundary weights, dense and sparse caches, and cache saturation.
- Every byte-channel transfer value matches the original calculation exactly.
- The seven existing end-to-end parallel-equivalence fixtures retained their
  original output digests in the before/after optimization comparison.
- Both final optimized Ewoks runs produced pixel SHA-256
  `ba6c146fd2092ab50485c25e2e0774cc36102d15657c196270cc23694e6d1fc7`
  and optimization score `10.71353549413827`.

The original Ewoks run timed out before producing its final output. Therefore,
full Ewoks output identity against that run has not been established; exact
compatibility evidence comes from scorer comparisons and the completed
end-to-end regression fixtures. This work does not change or validate physical
print appearance.

Local profiles, frozen test executables, the settings snapshot, and result JSON
are in ignored `artifacts/refinement-performance`. The temporary local profiling
test is preserved there as `refinement_local_test.go.txt` and is not part of the
normal test suite. Reproduce the portable boundary benchmark with:

```powershell
. .\scripts\env.ps1
go test ./internal/engine -run '^$' -bench '^BenchmarkStackSurfaceReuse$' -benchmem
```
