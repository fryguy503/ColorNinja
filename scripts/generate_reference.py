"""Refresh committed Go fixtures using the original Python implementation.

This is development-only validation. The Go CLI and desktop app do not invoke
Python or depend on these packages at runtime.
"""
from pathlib import Path
import importlib.util
import json
import sys
import numpy as np
from PIL import Image

root = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("reference", root / "perceptual_color_reduce.py")
module = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = module
spec.loader.exec_module(module)
out = root / "internal" / "engine" / "testdata"
out.mkdir(parents=True, exist_ok=True)
w, h = 96, 64
x = np.broadcast_to(np.linspace(0, 255, w, dtype=np.uint8), (h, w))
y = np.broadcast_to(np.linspace(0, 255, h, dtype=np.uint8)[:, None], (h, w))
rgba = np.dstack((x, y, ((x.astype(np.uint16) + y) // 2).astype(np.uint8), x))
source = Image.fromarray(rgba, "RGBA")
source.save(out / "gradient.png")
records = []
for name, rgb, td in [("Black", "#000000", .5), ("White", "#FFFFFF", 1), ("Red", "#FF0000", .8), ("Blue", "#0000FF", 2)]:
    records.append(dict(Brand="Test", Name=name, Color=rgb, Transmissivity=td, Type="PLA", Owned=True, uuid=name))
(out / "library.json").write_text(json.dumps({"Filaments": records}, indent=2))
library = module.load_hueforge_library(out / "library.json")
refs = []
for mode, sigma, limit in [("standard", 0, 0), ("standard", 1.5, 0), ("standard", 1.5, 1000), ("guided", 0, 0), ("stack", 0, 0)]:
    options = module.ReductionOptions(population_colors=4, analysis_max_pixels=limit, preblur_sigma=sigma, min_cluster_fraction=0, histogram_bits=5, max_iterations=12)
    hf = module.HueForgeOptions(layer_height_mm=.08, base_depth_mm=.48, max_depth_mm=.8, analysis_colors=8, beam_width=6, max_perceived_colors=12)
    if mode == "standard":
        result = module.reduce_image(source, options)
    elif mode == "guided":
        result = module.reduce_image_hueforge_guided(source, options, hf, library)
    else:
        result = module.reduce_image_hueforge(source, options, hf, library)
    name = f"{mode}-blur{sigma}-limit{limit}"
    Image.fromarray(result.rgba, "RGBA").save(out / (name + ".png"))
    refs.append(dict(name=name, mode=mode, sigma=sigma, limit=limit, palette=[list(p.rgb) for p in result.palette], mean=result.quality.mean_delta_e76, rms=result.quality.rms_delta_e76, analysis=list(result.analysis_size)))
(out / "reference.json").write_text(json.dumps(refs, indent=2))
print(f"Wrote {len(refs)} reference cases to {out}")

adobe_path = out / "adobergb.jpg"
if adobe_path.exists():
    adobe = Image.open(adobe_path)
    profile = adobe.info["icc_profile"]
    source.save(out / "adobe-transparent.png", icc_profile=profile)
    converted = module.load_image(out / "adobe-transparent.png")
    converted.rgba.save(out / "adobe-srgb-reference.png")
    print("Wrote Adobe RGB / alpha color-management fixtures")
