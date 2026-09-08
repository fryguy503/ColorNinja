#!/usr/bin/env python3
"""Reduce an image to a compact perceptual palette at full resolution.

This is an independent implementation of the observable ColorSmith-style
pipeline.  It does not include or load ColorSmith's JavaScript or WebAssembly
engine.  Palette discovery uses a filtered analysis image, separate chromatic
and achromatic populations, deterministic weighted clustering in CIE Lab, and
small-cluster culling.  The learned palette is then applied to every pixel of
the original-resolution image without dithering.
"""

from __future__ import annotations

import argparse
from dataclasses import dataclass, replace
from io import BytesIO
import hashlib
from itertools import combinations
import json
import math
import os
from pathlib import Path
import re
import sys
import tempfile
import time
from typing import Optional, Sequence

try:
    import numpy as np
except ImportError as error:  # pragma: no cover - depends on caller environment
    raise SystemExit(
        "NumPy is required. Install dependencies with: "
        "python -m pip install numpy Pillow"
    ) from error

try:
    from PIL import Image, ImageCms, ImageFilter, ImageOps, UnidentifiedImageError
except ImportError as error:  # pragma: no cover - depends on caller environment
    raise SystemExit(
        "Pillow is required. Install dependencies with: "
        "python -m pip install numpy Pillow"
    ) from error


DEFAULT_ANALYSIS_MAX_PIXELS = 6_291_456
DEFAULT_POPULATION_COLORS = 32
DEFAULT_CHUNK_PIXELS = 131_072
DEFAULT_HUEFORGE_FILAMENTS = 4
DEFAULT_HUEFORGE_ANALYSIS_COLORS = 32
DEFAULT_HUEFORGE_LAYER_HEIGHT_MM = 0.08
DEFAULT_HUEFORGE_BASE_DEPTH_MM = 0.48
DEFAULT_HUEFORGE_MAX_DEPTH_MM = 2.24
DEFAULT_HUEFORGE_BEAM_WIDTH = 24
DEFAULT_HUEFORGE_MAX_PERCEIVED_COLORS = 64
DEFAULT_HUEFORGE_TD_TRANSMISSION = 0.05
DEFAULT_HUEFORGE_TD_SCALE = 0.1
DEFAULT_HUEFORGE_BASE_TRANSMISSION_LIMIT = 0.10
DEFAULT_HUEFORGE_GUIDANCE_STRENGTH = 0.80
MAX_HUEFORGE_LAYERS = 4096
HUEFORGE_STACK_MODEL = "independent-frontlit-scaled-td-linear-srgb-v1"
HUEFORGE_GUIDANCE_MODEL = "inventory-guided-pairwise-td-hues-v1"
HEX_COLOR_PATTERN = re.compile(r"^#?([0-9A-Fa-f]{6})(?:[0-9A-Fa-f]{2})?$")


class ReductionError(RuntimeError):
    """Raised for an input, processing, or output failure."""


@dataclass(frozen=True)
class ReductionOptions:
    """Tunable palette-discovery and full-resolution mapping settings."""

    population_colors: int = DEFAULT_POPULATION_COLORS
    analysis_max_pixels: int = DEFAULT_ANALYSIS_MAX_PIXELS
    neutral_chroma: float = 8.0
    min_cluster_fraction: float = 0.005
    histogram_bits: int = 6
    max_iterations: int = 24
    chunk_pixels: int = DEFAULT_CHUNK_PIXELS
    preblur_sigma: float = 1.5

    def validate(self) -> None:
        if not 1 <= self.population_colors <= 256:
            raise ReductionError("population color budget must be between 1 and 256")
        if self.analysis_max_pixels < 0:
            raise ReductionError("analysis maximum pixels cannot be negative")
        if not math.isfinite(self.neutral_chroma) or self.neutral_chroma < 0:
            raise ReductionError(
                "neutral chroma threshold must be finite and non-negative"
            )
        if not math.isfinite(self.min_cluster_fraction) or not (
            0 <= self.min_cluster_fraction < 1
        ):
            raise ReductionError("minimum cluster fraction must be in [0, 1)")
        if not 3 <= self.histogram_bits <= 7:
            raise ReductionError("histogram bits must be between 3 and 7")
        if self.max_iterations < 1:
            raise ReductionError("maximum iterations must be positive")
        if self.chunk_pixels < 1:
            raise ReductionError("chunk size must be positive")
        if not math.isfinite(self.preblur_sigma) or not (
            0 <= self.preblur_sigma <= 100
        ):
            raise ReductionError("pre-blur sigma must be finite and in [0, 100]")


@dataclass(frozen=True)
class HueForgeOptions:
    """Settings for inventory-aware hue simulation and stack optimization."""

    layer_height_mm: float = DEFAULT_HUEFORGE_LAYER_HEIGHT_MM
    base_depth_mm: float = DEFAULT_HUEFORGE_BASE_DEPTH_MM
    max_depth_mm: float = DEFAULT_HUEFORGE_MAX_DEPTH_MM
    analysis_colors: int = DEFAULT_HUEFORGE_ANALYSIS_COLORS
    beam_width: int = DEFAULT_HUEFORGE_BEAM_WIDTH
    max_perceived_colors: int = DEFAULT_HUEFORGE_MAX_PERCEIVED_COLORS
    td_transmission: float = DEFAULT_HUEFORGE_TD_TRANSMISSION
    td_scale: float = DEFAULT_HUEFORGE_TD_SCALE
    base_transmission_limit: float = DEFAULT_HUEFORGE_BASE_TRANSMISSION_LIMIT

    def validate(self) -> None:
        for name, value in (
            ("layer height", self.layer_height_mm),
            ("base depth", self.base_depth_mm),
            ("maximum depth", self.max_depth_mm),
        ):
            if not math.isfinite(value) or value <= 0:
                raise ReductionError(f"HueForge {name} must be finite and positive")
        if self.max_depth_mm <= self.base_depth_mm:
            raise ReductionError(
                "HueForge maximum depth must be greater than the base depth"
            )
        if not 1 <= self.analysis_colors <= 256:
            raise ReductionError(
                "HueForge analysis colors must be between 1 and 256"
            )
        if not 1 <= self.beam_width <= 512:
            raise ReductionError("HueForge beam width must be between 1 and 512")
        if not 1 <= self.max_perceived_colors <= 256:
            raise ReductionError(
                "HueForge perceived color limit must be between 1 and 256"
            )
        if not math.isfinite(self.td_transmission) or not (
            0 < self.td_transmission < 1
        ):
            raise ReductionError(
                "HueForge TD transmission must be finite and between 0 and 1"
            )
        if not math.isfinite(self.td_scale) or self.td_scale <= 0:
            raise ReductionError(
                "HueForge front-lit TD scale must be finite and positive"
            )
        if not math.isfinite(self.base_transmission_limit) or not (
            0 < self.base_transmission_limit <= 1
        ):
            raise ReductionError(
                "HueForge base transmission limit must be in (0, 1]"
            )

        base_layers = self.base_depth_mm / self.layer_height_mm
        maximum_layers = self.max_depth_mm / self.layer_height_mm
        if not math.isfinite(base_layers) or not math.isfinite(maximum_layers):
            raise ReductionError("HueForge layer counts are too large")
        rounded_base_layers = int(round(base_layers))
        rounded_maximum_layers = int(round(maximum_layers))
        if not math.isclose(
            base_layers, rounded_base_layers, rel_tol=0.0, abs_tol=1e-8
        ):
            raise ReductionError(
                "HueForge base depth must be an exact multiple of layer height"
            )
        if not math.isclose(
            maximum_layers,
            rounded_maximum_layers,
            rel_tol=0.0,
            abs_tol=1e-8,
        ):
            raise ReductionError(
                "HueForge maximum depth must be an exact multiple of layer height"
            )
        if rounded_base_layers < 1:
            raise ReductionError("HueForge base depth must contain at least one layer")
        if rounded_maximum_layers <= rounded_base_layers:
            raise ReductionError(
                "HueForge maximum depth must leave at least one layer above the base"
            )
        if rounded_maximum_layers > MAX_HUEFORGE_LAYERS:
            raise ReductionError(
                f"HueForge plan cannot exceed {MAX_HUEFORGE_LAYERS} layers"
            )

    @property
    def base_layers(self) -> int:
        return int(round(self.base_depth_mm / self.layer_height_mm))

    @property
    def transition_layers(self) -> int:
        return int(
            round(
                (self.max_depth_mm - self.base_depth_mm) / self.layer_height_mm
            )
        )


@dataclass(frozen=True)
class HueForgeFilament:
    brand: str
    name: str
    rgb: tuple[int, int, int]
    td_mm: float
    material_type: str
    uuid: str
    owned: bool
    tags: tuple[str, ...]
    source_index: int
    has_secondary_color: bool = False

    @property
    def hex_color(self) -> str:
        return "#{:02X}{:02X}{:02X}".format(*self.rgb)


@dataclass(frozen=True)
class HueForgeLibrary:
    filaments: tuple[HueForgeFilament, ...]
    total_entries: int
    skipped_unowned: int
    skipped_filtered: int
    skipped_invalid: int
    skipped_secondary: int
    skipped_duplicate: int
    sha256: str


@dataclass(frozen=True)
class HueForgeStackRun:
    position: int
    filament: HueForgeFilament
    layers: int
    start_layer: int
    end_layer: int
    start_height_mm: float
    end_height_mm: float


@dataclass(frozen=True)
class HueForgeLayerColor:
    layer: int
    height_mm: float
    rgb: tuple[int, int, int]
    top_filament_position: int
    analysis_fraction: float = 0.0


@dataclass(frozen=True)
class HueForgeStackPlan:
    model: str
    td_transmission: float
    td_scale: float
    base_transmission_limit: float
    requested_filaments: int
    eligible_filaments: int
    eligible_base_filaments: int
    layer_height_mm: float
    base_depth_mm: float
    max_depth_mm: float
    planned_depth_mm: float
    runs: tuple[HueForgeStackRun, ...]
    layer_colors: tuple[HueForgeLayerColor, ...]
    weighted_rms_delta_e76: float
    library_sha256: str
    search_method: str = "deterministic-multisize-beam-with-capped-terminal-rerank"


@dataclass(frozen=True)
class HueForgeGuidedColor:
    rgb: tuple[int, int, int]
    reference_rgb: tuple[int, int, int]
    reference_kind: str
    filament_positions: tuple[int, ...]
    top_layers: int
    analysis_fraction: float

    @property
    def hex_color(self) -> str:
        return "#{:02X}{:02X}{:02X}".format(*self.rgb)


@dataclass(frozen=True)
class HueForgeGuidancePlan:
    model: str
    td_transmission: float
    td_scale: float
    layer_height_mm: float
    maximum_pairwise_top_layers: int
    guidance_strength: float
    requested_filaments: int
    eligible_filaments: int
    selected_filaments: tuple[HueForgeFilament, ...]
    candidate_color_count: int
    colors: tuple[HueForgeGuidedColor, ...]
    weighted_rms_delta_e76: float
    library_sha256: str
    search_method: str = (
        "deterministic-greedy-anchor-selection-with-local-swaps-and-"
        "unused-anchor-pruning"
    )


@dataclass(frozen=True)
class _GuidanceCandidate:
    rgb: tuple[int, int, int]
    kind: str
    filament_indices: tuple[int, ...]
    top_layers: int = 0


@dataclass
class _StackSearchState:
    selected_indices: tuple[int, ...]
    run_layers: tuple[int, ...]
    used_transition_layers: int
    current_linear: np.ndarray
    stack_rgbs: tuple[tuple[int, int, int], ...]
    stack_layers: tuple[int, ...]
    stack_positions: tuple[int, ...]
    best_distances: np.ndarray
    score: float


@dataclass(frozen=True)
class PaletteEntry:
    rgb: tuple[int, int, int]
    population: str
    analysis_fraction: float
    stack_layer: int | None = None
    stack_height_mm: float | None = None
    top_filament_position: int | None = None

    @property
    def hex_color(self) -> str:
        return "#{:02X}{:02X}{:02X}".format(*self.rgb)


@dataclass(frozen=True)
class QualityMetrics:
    mean_delta_e76: float
    rms_delta_e76: float
    max_delta_e76: float


@dataclass(frozen=True)
class ReductionResult:
    rgba: np.ndarray
    palette: tuple[PaletteEntry, ...]
    source_size: tuple[int, int]
    analysis_size: tuple[int, int]
    quality: QualityMetrics
    rgba_sha256: str
    hueforge_plan: HueForgeStackPlan | None = None
    hueforge_guidance: HueForgeGuidancePlan | None = None
    stack_layer_map: np.ndarray | None = None


@dataclass(frozen=True)
class LoadedImage:
    rgba: Image.Image
    had_alpha: bool
    save_metadata: dict[str, object]


def _js_round_positive(value: float) -> int:
    """Match JavaScript Math.round for the positive dimensions used here."""

    return int(math.floor(value + 0.5))


def compute_analysis_size(
    width: int, height: int, maximum_pixels: int
) -> tuple[int, int]:
    """Return the filtered-analysis dimensions while preserving aspect ratio."""

    if width < 1 or height < 1:
        raise ReductionError("image dimensions must be positive")
    if maximum_pixels < 0:
        raise ReductionError("analysis maximum pixels cannot be negative")
    if maximum_pixels == 0 or width * height <= maximum_pixels:
        return width, height

    scale = math.sqrt(maximum_pixels / float(width * height))
    analysis_width = max(1, _js_round_positive(width * scale))
    analysis_height = max(1, _js_round_positive(height * scale))
    # Independent rounding can put the product just above the requested cap,
    # and an extreme aspect ratio can round its short side to zero before the
    # clamp. Tighten only the longer side to make "maximum" a real invariant.
    if analysis_width * analysis_height > maximum_pixels:
        if analysis_width >= analysis_height:
            analysis_width = max(1, maximum_pixels // analysis_height)
        else:
            analysis_height = max(1, maximum_pixels // analysis_width)
    return analysis_width, analysis_height


def _srgb_to_lab(rgb: np.ndarray) -> np.ndarray:
    """Convert sRGB bytes/floats in [0, 255] to CIE Lab (D65)."""

    values = np.asarray(rgb, dtype=np.float32) / np.float32(255.0)
    linear = np.where(
        values <= np.float32(0.04045),
        values / np.float32(12.92),
        ((values + np.float32(0.055)) / np.float32(1.055))
        ** np.float32(2.4),
    )

    xyz = linear @ np.asarray(
        (
            (0.4124564, 0.3575761, 0.1804375),
            (0.2126729, 0.7151522, 0.0721750),
            (0.0193339, 0.1191920, 0.9503041),
        ),
        dtype=np.float32,
    ).T
    xyz /= np.asarray((0.95047, 1.0, 1.08883), dtype=np.float32)

    delta = np.float32(6.0 / 29.0)
    threshold = delta**3
    transformed = np.where(
        xyz > threshold,
        np.cbrt(xyz),
        xyz / (np.float32(3.0) * delta**2) + np.float32(4.0 / 29.0),
    )
    fx, fy, fz = np.moveaxis(transformed, -1, 0)
    return np.stack(
        (
            np.float32(116.0) * fy - np.float32(16.0),
            np.float32(500.0) * (fx - fy),
            np.float32(200.0) * (fy - fz),
        ),
        axis=-1,
    ).astype(np.float32, copy=False)


def _lab_to_srgb(lab: np.ndarray) -> np.ndarray:
    """Convert CIE Lab (D65) values to clipped, rounded sRGB bytes."""

    values = np.asarray(lab, dtype=np.float64)
    lightness, channel_a, channel_b = np.moveaxis(values, -1, 0)
    fy = (lightness + 16.0) / 116.0
    fx = fy + channel_a / 500.0
    fz = fy - channel_b / 200.0
    f_values = np.stack((fx, fy, fz), axis=-1)

    delta = 6.0 / 29.0
    xyz = np.where(
        f_values > delta,
        f_values**3,
        3.0 * delta**2 * (f_values - 4.0 / 29.0),
    )
    xyz *= np.asarray((0.95047, 1.0, 1.08883), dtype=np.float64)
    linear = xyz @ np.asarray(
        (
            (3.2404542, -1.5371385, -0.4985314),
            (-0.9692660, 1.8760108, 0.0415560),
            (0.0556434, -0.2040259, 1.0572252),
        ),
        dtype=np.float64,
    ).T
    linear = np.clip(linear, 0.0, 1.0)
    srgb = np.where(
        linear <= 0.0031308,
        12.92 * linear,
        1.055 * np.power(linear, 1.0 / 2.4) - 0.055,
    )
    return np.floor(np.clip(srgb, 0.0, 1.0) * 255.0 + 0.5).astype(np.uint8)


def _convert_embedded_profile_to_srgb(image: Image.Image) -> tuple[Image.Image, bytes | None]:
    profile_bytes = image.info.get("icc_profile")
    if not profile_bytes:
        return image, None

    try:
        source_profile = ImageCms.ImageCmsProfile(BytesIO(profile_bytes))
        destination_profile = ImageCms.createProfile("sRGB")
        alpha = image.convert("RGBA").getchannel("A")
        converted = ImageCms.profileToProfile(
            image,
            source_profile,
            destination_profile,
            outputMode="RGB",
        ).convert("RGBA")
        converted.putalpha(alpha)
        output_profile = ImageCms.ImageCmsProfile(destination_profile).tobytes()
        return converted, output_profile
    except (ImageCms.PyCMSError, OSError, ValueError) as error:
        print(
            f"warning: could not apply embedded ICC profile; assuming sRGB ({error})",
            file=sys.stderr,
        )
        return image, None


def load_image(path: Path) -> LoadedImage:
    """Load the first frame, apply EXIF orientation, and normalize it to RGBA/sRGB."""

    try:
        with Image.open(path) as opened:
            opened.seek(0)
            opened.load()
            had_alpha = "A" in opened.getbands() or "transparency" in opened.info
            dpi = opened.info.get("dpi")
            oriented = ImageOps.exif_transpose(opened)
            converted, output_profile = _convert_embedded_profile_to_srgb(oriented)
            rgba = converted.convert("RGBA").copy()
    except (
        FileNotFoundError,
        PermissionError,
        OSError,
        UnidentifiedImageError,
        Image.DecompressionBombError,
    ) as error:
        raise ReductionError(f"could not read image {path}: {error}") from error

    metadata: dict[str, object] = {}
    if output_profile:
        metadata["icc_profile"] = output_profile
    if dpi and len(dpi) == 2:
        metadata["dpi"] = dpi
    return LoadedImage(rgba=rgba, had_alpha=had_alpha, save_metadata=metadata)


def _srgb_to_linear(rgb: np.ndarray | Sequence[float]) -> np.ndarray:
    """Convert sRGB values in [0, 255] to linear-light RGB in [0, 1]."""

    values = np.asarray(rgb, dtype=np.float64) / 255.0
    return np.where(
        values <= 0.04045,
        values / 12.92,
        ((values + 0.055) / 1.055) ** 2.4,
    )


def _linear_to_srgb(linear: np.ndarray | Sequence[float]) -> np.ndarray:
    """Convert linear-light RGB in [0, 1] to rounded sRGB bytes."""

    values = np.clip(np.asarray(linear, dtype=np.float64), 0.0, 1.0)
    srgb = np.where(
        values <= 0.0031308,
        12.92 * values,
        1.055 * np.power(values, 1.0 / 2.4) - 0.055,
    )
    return np.floor(np.clip(srgb, 0.0, 1.0) * 255.0 + 0.5).astype(np.uint8)


def _parse_hex_color(value: object) -> tuple[int, int, int] | None:
    if not isinstance(value, str):
        return None
    match = HEX_COLOR_PATTERN.fullmatch(value.strip())
    if match is None:
        return None
    digits = match.group(1)
    return tuple(int(digits[offset : offset + 2], 16) for offset in (0, 2, 4))


def _library_bool(value: object) -> bool | None:
    if value is None:
        return None
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)) and not isinstance(value, bool):
        return value != 0
    if isinstance(value, str):
        normalized = value.strip().casefold()
        if normalized in {"true", "yes", "y", "1", "owned"}:
            return True
        if normalized in {"false", "no", "n", "0", "unowned"}:
            return False
    return None


def load_hueforge_library(
    path: Path,
    *,
    include_unowned: bool = False,
    material_types: Sequence[str] = (),
    allow_secondary: bool = False,
) -> HueForgeLibrary:
    """Load eligible primary colors from an exported HueForge filament library."""

    try:
        raw = path.read_bytes()
    except (FileNotFoundError, PermissionError, OSError) as error:
        raise ReductionError(
            f"could not read HueForge filament library {path}: {error}"
        ) from error

    try:
        document = json.loads(raw.decode("utf-8-sig"))
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise ReductionError(
            f"could not parse HueForge filament library {path}: {error}"
        ) from error

    if isinstance(document, list):
        records = document
    elif isinstance(document, dict):
        records = document.get("Filaments")
        if records is None:
            records = document.get("filaments")
    else:
        records = None
    if not isinstance(records, list):
        raise ReductionError(
            "HueForge filament library must contain a 'Filaments' array"
        )

    requested_types = {
        str(value).strip().casefold()
        for value in material_types
        if str(value).strip()
    }
    skipped_unowned = 0
    skipped_filtered = 0
    skipped_invalid = 0
    skipped_secondary = 0
    skipped_duplicate = 0
    filaments: list[HueForgeFilament] = []
    optical_keys: set[tuple[tuple[int, int, int], float, str]] = set()

    for source_index, record in enumerate(records):
        if not isinstance(record, dict):
            skipped_invalid += 1
            continue

        owned = _library_bool(record.get("Owned"))
        if owned is None:
            skipped_invalid += 1
            continue
        if not owned and not include_unowned:
            skipped_unowned += 1
            continue

        material_type_value = record.get("Type", "")
        material_type = (
            material_type_value.strip()
            if isinstance(material_type_value, str)
            else str(material_type_value).strip()
        )
        if requested_types and material_type.casefold() not in requested_types:
            skipped_filtered += 1
            continue

        secondary_value = record.get("Secondary_Color")
        has_secondary = (
            isinstance(secondary_value, str) and bool(secondary_value.strip())
        )
        if has_secondary and not allow_secondary:
            skipped_secondary += 1
            continue

        rgb = _parse_hex_color(record.get("Color"))
        try:
            td_mm = float(record.get("Transmissivity"))
        except (TypeError, ValueError):
            td_mm = math.nan
        if rgb is None or not math.isfinite(td_mm) or td_mm <= 0:
            skipped_invalid += 1
            continue

        optical_key = (rgb, round(td_mm, 9), material_type.casefold())
        if optical_key in optical_keys:
            skipped_duplicate += 1
            continue
        optical_keys.add(optical_key)

        brand_value = record.get("Brand", "")
        name_value = record.get("Name", "")
        uuid_value = record.get("uuid", "")
        brand = str(brand_value).strip() if brand_value is not None else ""
        name = str(name_value).strip() if name_value is not None else ""
        uuid = str(uuid_value).strip() if uuid_value is not None else ""
        if not brand:
            brand = "Unknown brand"
        if not name:
            name = f"Filament {source_index + 1}"
        tags_value = record.get("Tags", ())
        if isinstance(tags_value, str):
            tags = (tags_value.strip(),) if tags_value.strip() else ()
        elif isinstance(tags_value, (list, tuple)):
            tags = tuple(
                str(tag).strip()
                for tag in tags_value
                if str(tag).strip()
            )
        else:
            tags = ()

        filaments.append(
            HueForgeFilament(
                brand=brand,
                name=name,
                rgb=rgb,
                td_mm=td_mm,
                material_type=material_type,
                uuid=uuid,
                owned=owned,
                tags=tags,
                source_index=source_index,
                has_secondary_color=has_secondary,
            )
        )

    if not filaments:
        raise ReductionError(
            "HueForge filament library has no eligible primary-color filaments "
            "after applying ownership, type, and secondary-color filters"
        )
    return HueForgeLibrary(
        filaments=tuple(filaments),
        total_entries=len(records),
        skipped_unowned=skipped_unowned,
        skipped_filtered=skipped_filtered,
        skipped_invalid=skipped_invalid,
        skipped_secondary=skipped_secondary,
        skipped_duplicate=skipped_duplicate,
        sha256=hashlib.sha256(raw).hexdigest(),
    )


def _preblurred_analysis_rgb(
    image: Image.Image, sigma: float
) -> tuple[np.ndarray, np.ndarray]:
    """Blur premultiplied RGB for palette discovery and return original alpha."""

    rgba_image = image if image.mode == "RGBA" else image.convert("RGBA")
    rgba = np.asarray(rgba_image, dtype=np.uint8)
    alpha = rgba[..., 3]
    if sigma <= 0:
        rgb = rgba[..., :3].copy()
        rgb[alpha == 0] = 0
        return rgb, alpha

    if np.all(alpha == 255):
        blurred = image.convert("RGB").filter(ImageFilter.GaussianBlur(sigma))
        return np.asarray(blurred, dtype=np.uint8), alpha

    alpha_f = alpha.astype(np.float32) / np.float32(255.0)
    premultiplied = np.floor(
        rgba[..., :3].astype(np.float32) * alpha_f[..., None] + 0.5
    ).astype(np.uint8)
    blurred_rgb = np.asarray(
        Image.fromarray(premultiplied, mode="RGB").filter(
            ImageFilter.GaussianBlur(sigma)
        ),
        dtype=np.float32,
    )
    blurred_alpha = np.asarray(
        Image.fromarray(alpha, mode="L").filter(ImageFilter.GaussianBlur(sigma)),
        dtype=np.float32,
    )
    safe_alpha = np.maximum(blurred_alpha, np.float32(1.0))
    unpremultiplied = np.clip(
        blurred_rgb * np.float32(255.0) / safe_alpha[..., None], 0.0, 255.0
    )
    result = np.floor(unpremultiplied + 0.5).astype(np.uint8)
    result[blurred_alpha < 0.5] = 0
    return result, alpha


def _weighted_rgb_histogram(
    rgb: np.ndarray, alpha: np.ndarray, bits: int
) -> tuple[np.ndarray, np.ndarray]:
    """Compress pixels into stable RGB bins and return mean RGB plus alpha mass."""

    bin_count = 1 << (3 * bits)
    shift = 8 - bits
    counts = np.zeros(bin_count, dtype=np.float64)
    sums = np.zeros((bin_count, 3), dtype=np.float64)
    flat_rgb = np.asarray(rgb, dtype=np.uint8).reshape(-1, 3)
    flat_alpha = np.asarray(alpha, dtype=np.uint8).reshape(-1)

    histogram_chunk = 1_048_576
    for start in range(0, flat_rgb.shape[0], histogram_chunk):
        stop = min(start + histogram_chunk, flat_rgb.shape[0])
        chunk_rgb = flat_rgb[start:stop]
        chunk_alpha = flat_alpha[start:stop].astype(np.float64) / 255.0
        visible = chunk_alpha > 0
        if not np.any(visible):
            continue

        visible_rgb = chunk_rgb[visible]
        visible_alpha = chunk_alpha[visible]
        codes = (
            (visible_rgb[:, 0].astype(np.int64) >> shift) << (2 * bits)
        ) | ((visible_rgb[:, 1].astype(np.int64) >> shift) << bits) | (
            visible_rgb[:, 2].astype(np.int64) >> shift
        )
        counts += np.bincount(
            codes, weights=visible_alpha, minlength=bin_count
        )
        for channel in range(3):
            sums[:, channel] += np.bincount(
                codes,
                weights=visible_rgb[:, channel].astype(np.float64)
                * visible_alpha,
                minlength=bin_count,
            )

    occupied = counts > 0
    if not np.any(occupied):
        return np.asarray(((0.0, 0.0, 0.0),)), np.asarray((1.0,))
    return sums[occupied] / counts[occupied, None], counts[occupied]


def _assign_to_centers(
    points: np.ndarray, centers: np.ndarray
) -> tuple[np.ndarray, np.ndarray]:
    labels = np.zeros(points.shape[0], dtype=np.int32)
    best_distance = np.full(points.shape[0], np.inf, dtype=np.float64)
    for index, center in enumerate(centers):
        difference = points - center
        distance = np.einsum("ij,ij->i", difference, difference, optimize=True)
        closer = distance < best_distance
        labels[closer] = index
        best_distance[closer] = distance[closer]
    return labels, best_distance


def _initial_centers(
    points: np.ndarray, weights: np.ndarray, requested: int
) -> np.ndarray:
    """Choose stable weighted farthest-point seeds without random state."""

    count = min(requested, points.shape[0])
    weighted_mean = np.average(points, axis=0, weights=weights)
    difference = points - weighted_mean
    mean_distance = np.einsum(
        "ij,ij->i", difference, difference, optimize=True
    )
    first = int(np.argmin(mean_distance))
    chosen = [first]
    difference = points - points[first]
    nearest = np.einsum("ij,ij->i", difference, difference, optimize=True)

    while len(chosen) < count:
        score = weights * nearest
        score[np.asarray(chosen, dtype=np.int64)] = -1.0
        candidate = int(np.argmax(score))
        if score[candidate] <= 0:
            remaining = np.setdiff1d(
                np.arange(points.shape[0]), np.asarray(chosen), assume_unique=True
            )
            if remaining.size == 0:
                break
            candidate = int(remaining[0])
        chosen.append(candidate)
        difference = points - points[candidate]
        distance = np.einsum(
            "ij,ij->i", difference, difference, optimize=True
        )
        nearest = np.minimum(nearest, distance)
    return points[np.asarray(chosen, dtype=np.int64)].astype(np.float64, copy=True)


def _refine_centers(
    points: np.ndarray,
    weights: np.ndarray,
    centers: np.ndarray,
    maximum_iterations: int,
) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
    previous_labels: np.ndarray | None = None
    centers = np.asarray(centers, dtype=np.float64).copy()

    for _ in range(maximum_iterations):
        labels, best_distance = _assign_to_centers(points, centers)
        masses = np.bincount(
            labels, weights=weights, minlength=centers.shape[0]
        ).astype(np.float64)
        updated = centers.copy()
        for channel in range(points.shape[1]):
            channel_sums = np.bincount(
                labels,
                weights=weights * points[:, channel],
                minlength=centers.shape[0],
            )
            nonempty = masses > 0
            updated[nonempty, channel] = channel_sums[nonempty] / masses[nonempty]

        empty = np.flatnonzero(masses == 0)
        if empty.size:
            errors = weights * best_distance
            used: set[int] = set()
            for center_index in empty:
                candidate = int(np.argmax(errors))
                while candidate in used:
                    errors[candidate] = -1.0
                    candidate = int(np.argmax(errors))
                updated[center_index] = points[candidate]
                used.add(candidate)
                errors[candidate] = -1.0

        stable_labels = previous_labels is not None and np.array_equal(
            labels, previous_labels
        )
        movement = float(np.max(np.linalg.norm(updated - centers, axis=1)))
        centers = updated
        previous_labels = labels
        if stable_labels or movement < 1e-5:
            break

    labels, _ = _assign_to_centers(points, centers)
    masses = np.bincount(
        labels, weights=weights, minlength=centers.shape[0]
    ).astype(np.float64)
    return centers, labels, masses


def _cluster_population(
    points: np.ndarray,
    weights: np.ndarray,
    requested: int,
    minimum_fraction: float,
    maximum_iterations: int,
) -> tuple[np.ndarray, np.ndarray]:
    if points.shape[0] == 0:
        return np.empty((0, 3), dtype=np.float64), np.empty(0, dtype=np.float64)

    centers = _initial_centers(points, weights, requested)
    centers, _, masses = _refine_centers(
        points, weights, centers, maximum_iterations
    )
    minimum_mass = float(weights.sum()) * minimum_fraction
    # Refining the survivors can move a boundary and make another cluster fall
    # below the same threshold. Repeat until the advertised cull invariant is
    # actually true; every pass removes at least one center, so this terminates.
    while centers.shape[0] > 1:
        keep = np.flatnonzero(masses >= minimum_mass)
        if keep.size == centers.shape[0]:
            break
        if keep.size == 0:
            keep = np.asarray((int(np.argmax(masses)),), dtype=np.int64)
        centers, _, masses = _refine_centers(
            points, weights, centers[keep], min(8, maximum_iterations)
        )
    return centers, masses


def discover_palette(
    source: Image.Image, options: ReductionOptions
) -> tuple[tuple[PaletteEntry, ...], tuple[int, int]]:
    """Learn the palette from a filtered, possibly downscaled analysis image."""

    options.validate()
    source_width, source_height = source.size
    analysis_size = compute_analysis_size(
        source_width, source_height, options.analysis_max_pixels
    )
    if analysis_size == source.size:
        analysis = source
    else:
        analysis = source.resize(analysis_size, Image.Resampling.LANCZOS)

    scale = analysis_size[0] / float(source_width)
    sigma = 0.0
    if options.preblur_sigma > 0:
        sigma = max(0.5, min(options.preblur_sigma, options.preblur_sigma * scale))
    rgb, alpha = _preblurred_analysis_rgb(analysis, sigma)
    candidate_rgb, candidate_weights = _weighted_rgb_histogram(
        rgb, alpha, options.histogram_bits
    )
    candidate_lab = _srgb_to_lab(candidate_rgb).astype(np.float64)
    chroma = np.hypot(candidate_lab[:, 1], candidate_lab[:, 2])
    neutral_mask = chroma < options.neutral_chroma

    groups: list[tuple[str, np.ndarray, np.ndarray]] = []
    for name, mask in (
        ("chromatic", ~neutral_mask),
        ("achromatic", neutral_mask),
    ):
        centers, masses = _cluster_population(
            candidate_lab[mask],
            candidate_weights[mask],
            options.population_colors,
            options.min_cluster_fraction,
            options.max_iterations,
        )
        if centers.size:
            groups.append((name, centers, masses))

    total_mass = float(candidate_weights.sum())
    entries: list[PaletteEntry] = []
    seen: dict[tuple[int, int, int], int] = {}
    for population, centers, masses in groups:
        center_rgb = _lab_to_srgb(centers)
        order = np.argsort(-masses, kind="stable")
        for index in order:
            rgb_tuple = tuple(int(value) for value in center_rgb[index])
            fraction = float(masses[index] / total_mass) if total_mass else 0.0
            existing = seen.get(rgb_tuple)
            if existing is None:
                seen[rgb_tuple] = len(entries)
                entries.append(
                    PaletteEntry(
                        rgb=rgb_tuple,
                        population=population,
                        analysis_fraction=fraction,
                    )
                )
            else:
                old = entries[existing]
                entries[existing] = PaletteEntry(
                    rgb=old.rgb,
                    population=old.population,
                    analysis_fraction=old.analysis_fraction + fraction,
                )

    if not entries:
        entries.append(PaletteEntry((0, 0, 0), "achromatic", 1.0))
    return tuple(entries), analysis_size


def _simulate_hueforge_layer(
    current_linear: np.ndarray,
    filament: HueForgeFilament,
    options: HueForgeOptions,
) -> np.ndarray:
    """Apply one uniform top layer using ColorNinja's documented TD model."""

    effective_td_mm = filament.td_mm * options.td_scale
    transmission = options.td_transmission ** (
        options.layer_height_mm / effective_td_mm
    )
    top_linear = _srgb_to_linear(filament.rgb)
    return np.clip(
        (1.0 - transmission) * top_linear
        + transmission * np.asarray(current_linear, dtype=np.float64),
        0.0,
        1.0,
    )


def _stack_target_arrays(
    target_palette: Sequence[PaletteEntry],
) -> tuple[np.ndarray, np.ndarray]:
    target_rgb = np.asarray([entry.rgb for entry in target_palette], dtype=np.uint8)
    target_lab = _srgb_to_lab(target_rgb).astype(np.float64)
    target_weights = np.asarray(
        [max(0.0, entry.analysis_fraction) for entry in target_palette],
        dtype=np.float64,
    )
    weight_sum = float(target_weights.sum())
    if weight_sum <= 0:
        target_weights.fill(1.0 / len(target_weights))
    else:
        target_weights /= weight_sum
    return target_lab, target_weights


def _distance_squared_to_color(
    target_lab: np.ndarray, rgb: tuple[int, int, int]
) -> np.ndarray:
    color_lab = _srgb_to_lab(np.asarray((rgb,), dtype=np.uint8))[0].astype(
        np.float64
    )
    difference = target_lab - color_lab
    return np.einsum("ij,ij->i", difference, difference, optimize=True)


def _select_reachable_palette(
    candidate_lab: np.ndarray,
    target_lab: np.ndarray,
    target_weights: np.ndarray,
    maximum_colors: int,
    minimum_fraction: float,
) -> tuple[list[int], np.ndarray, float]:
    """Choose and repeatedly cull fixed, physically reachable stack colors."""

    difference = target_lab[:, None, :] - candidate_lab[None, :, :]
    distance = np.einsum("tci,tci->tc", difference, difference, optimize=True)
    limit = min(maximum_colors, candidate_lab.shape[0])

    def selection_score(indices: Sequence[int]) -> float:
        return float(np.dot(target_weights, np.min(distance[:, indices], axis=1)))

    candidate_count = candidate_lab.shape[0]
    combination_count = math.comb(candidate_count, limit)
    if limit == candidate_count:
        selected = list(range(candidate_count))
    elif limit <= 4 and combination_count <= 50_000:
        best_subset: tuple[int, ...] | None = None
        best_subset_score = math.inf
        for subset in combinations(range(candidate_count), limit):
            score = selection_score(subset)
            if score < best_subset_score - 1e-12:
                best_subset = subset
                best_subset_score = score
        selected = list(best_subset or (int(np.argmin(target_weights @ distance)),))
    else:
        best_selection: list[int] | None = None
        best_selection_score = math.inf
        for start in range(candidate_count):
            trial = [start]
            trial_score = selection_score(trial)
            while len(trial) < limit:
                addition = -1
                addition_score = trial_score
                trial_set = set(trial)
                for candidate in range(candidate_count):
                    if candidate in trial_set:
                        continue
                    score = selection_score(trial + [candidate])
                    if score < addition_score - 1e-12:
                        addition = candidate
                        addition_score = score
                if addition < 0:
                    break
                trial.append(addition)
                trial_score = addition_score

            improved = True
            while improved and trial:
                improved = False
                replacement_trial = trial
                replacement_score = trial_score
                trial_set = set(trial)
                for position in range(len(trial)):
                    for candidate in range(candidate_count):
                        if candidate in trial_set:
                            continue
                        proposal = trial.copy()
                        proposal[position] = candidate
                        proposal = sorted(proposal)
                        score = selection_score(proposal)
                        if score < replacement_score - 1e-12:
                            replacement_trial = proposal
                            replacement_score = score
                if replacement_score < trial_score - 1e-12:
                    trial = replacement_trial
                    trial_score = replacement_score
                    improved = True

            trial = sorted(trial)
            if (
                trial_score < best_selection_score - 1e-12
                or (
                    math.isclose(trial_score, best_selection_score, abs_tol=1e-12)
                    and (best_selection is None or tuple(trial) < tuple(best_selection))
                )
            ):
                best_selection = trial
                best_selection_score = trial_score
        selected = best_selection or [int(np.argmin(target_weights @ distance))]

    minimum_mass = float(target_weights.sum()) * minimum_fraction
    removed_candidates: set[int] = set()
    while len(selected) > 1:
        selected = sorted(selected)
        selected_distances = distance[:, selected]
        labels = np.argmin(selected_distances, axis=1)
        masses = np.bincount(
            labels, weights=target_weights, minlength=len(selected)
        ).astype(np.float64)
        keep = np.flatnonzero(
            (masses > 1e-15) & (masses >= minimum_mass)
        )
        if keep.size == len(selected):
            break
        if keep.size == 0:
            keep = np.asarray((int(np.argmax(masses)),), dtype=np.int64)
        kept_positions = {int(index) for index in keep}
        removed_candidates.update(
            candidate
            for index, candidate in enumerate(selected)
            if index not in kept_positions
        )
        selected = [selected[int(index)] for index in keep]

    while len(selected) < limit:
        current_score = selection_score(selected)
        best_addition = -1
        best_addition_score = current_score
        for candidate in range(candidate_count):
            if candidate in selected or candidate in removed_candidates:
                continue
            proposal = sorted(selected + [candidate])
            proposal_distances = distance[:, proposal]
            proposal_labels = np.argmin(proposal_distances, axis=1)
            proposal_masses = np.bincount(
                proposal_labels,
                weights=target_weights,
                minlength=len(proposal),
            )
            candidate_position = proposal.index(candidate)
            if proposal_masses[candidate_position] < max(minimum_mass, 1e-15):
                continue
            score = selection_score(proposal)
            if score < best_addition_score - 1e-12:
                best_addition = candidate
                best_addition_score = score
        if best_addition < 0:
            break
        selected.append(best_addition)

    while len(selected) > 1:
        selected = sorted(selected)
        selected_distances = distance[:, selected]
        labels = np.argmin(selected_distances, axis=1)
        masses = np.bincount(
            labels, weights=target_weights, minlength=len(selected)
        ).astype(np.float64)
        keep = np.flatnonzero(
            (masses > 1e-15) & (masses >= minimum_mass)
        )
        if keep.size == len(selected):
            break
        if keep.size == 0:
            keep = np.asarray((int(np.argmax(masses)),), dtype=np.int64)
        selected = [selected[int(index)] for index in keep]

    selected = sorted(selected)
    selected_distances = distance[:, selected]
    labels = np.argmin(selected_distances, axis=1)
    masses = np.bincount(
        labels, weights=target_weights, minlength=len(selected)
    ).astype(np.float64)
    best_distance = selected_distances[np.arange(target_lab.shape[0]), labels]
    rms_delta_e76 = math.sqrt(float(np.dot(target_weights, best_distance)))
    return selected, masses, rms_delta_e76


def _stack_state_output_score(
    state: _StackSearchState,
    target_lab: np.ndarray,
    target_weights: np.ndarray,
    maximum_colors: int,
    minimum_fraction: float,
) -> float:
    unique_rgbs = list(dict.fromkeys(state.stack_rgbs))
    candidate_lab = _srgb_to_lab(
        np.asarray(unique_rgbs, dtype=np.uint8)
    ).astype(np.float64)
    _, _, rms_delta_e76 = _select_reachable_palette(
        candidate_lab,
        target_lab,
        target_weights,
        maximum_colors,
        minimum_fraction,
    )
    return rms_delta_e76


def _generate_hueforge_guidance_candidates(
    filament_indices: Sequence[int],
    library: HueForgeLibrary,
    options: HueForgeOptions,
) -> tuple[_GuidanceCandidate, ...]:
    """Generate pure colors and TD-aware ordered pair hues for preprocessing."""

    indices = tuple(sorted(set(int(index) for index in filament_indices)))
    candidates: list[_GuidanceCandidate] = []
    seen: set[tuple[int, int, int]] = set()

    for filament_index in indices:
        rgb = library.filaments[filament_index].rgb
        if rgb not in seen:
            seen.add(rgb)
            candidates.append(
                _GuidanceCandidate(
                    rgb=rgb,
                    kind="filament",
                    filament_indices=(filament_index,),
                )
            )

    linear_colors = {
        index: _srgb_to_linear(library.filaments[index].rgb)
        for index in indices
    }
    layer_transmission = {
        index: options.td_transmission
        ** (
            options.layer_height_mm
            / (library.filaments[index].td_mm * options.td_scale)
        )
        for index in indices
    }
    for bottom_index in indices:
        for top_index in indices:
            if top_index == bottom_index:
                continue
            current_linear = linear_colors[bottom_index].copy()
            top_linear = linear_colors[top_index]
            transmission = layer_transmission[top_index]
            for top_layers in range(1, options.transition_layers + 1):
                current_linear = np.clip(
                    (1.0 - transmission) * top_linear
                    + transmission * current_linear,
                    0.0,
                    1.0,
                )
                rgb = tuple(int(value) for value in _linear_to_srgb(current_linear))
                if rgb in seen:
                    continue
                seen.add(rgb)
                candidates.append(
                    _GuidanceCandidate(
                        rgb=rgb,
                        kind="pairwise-perceived-hue",
                        filament_indices=(bottom_index, top_index),
                        top_layers=top_layers,
                    )
                )
    return tuple(candidates)


def _guidance_candidate_distance(
    candidates: Sequence[_GuidanceCandidate], target_lab: np.ndarray
) -> tuple[np.ndarray, np.ndarray]:
    candidate_lab = _srgb_to_lab(
        np.asarray([candidate.rgb for candidate in candidates], dtype=np.uint8)
    ).astype(np.float64)
    difference = target_lab[:, None, :] - candidate_lab[None, :, :]
    distance = np.einsum("tci,tci->tc", difference, difference, optimize=True)
    return candidate_lab, distance


def _guided_candidate_set_score(
    candidates: Sequence[_GuidanceCandidate],
    target_lab: np.ndarray,
    target_weights: np.ndarray,
    maximum_colors: int,
    minimum_fraction: float,
) -> float:
    candidate_lab, distance = _guidance_candidate_distance(candidates, target_lab)
    nearest = np.argmin(distance, axis=1)
    relevant = sorted({int(index) for index in nearest})
    if len(relevant) <= maximum_colors:
        best_distance = distance[
            np.arange(target_lab.shape[0]), nearest
        ]
        return float(np.dot(target_weights, best_distance))
    _, _, rms_delta_e76 = _select_reachable_palette(
        candidate_lab[relevant],
        target_lab,
        target_weights,
        maximum_colors,
        minimum_fraction,
    )
    return rms_delta_e76 * rms_delta_e76


def _optimize_hueforge_guidance(
    target_palette: Sequence[PaletteEntry],
    library: HueForgeLibrary,
    requested_filaments: int,
    reduction_options: ReductionOptions,
    hueforge_options: HueForgeOptions,
    guidance_strength: float = DEFAULT_HUEFORGE_GUIDANCE_STRENGTH,
) -> tuple[tuple[PaletteEntry, ...], HueForgeGuidancePlan]:
    """Select owned anchors and pull image colors toward their modeled hues."""

    if not target_palette:
        raise ReductionError("HueForge library guidance needs a target palette")
    if requested_filaments < 1:
        raise ReductionError("HueForge filament anchor budget must be positive")
    if not library.filaments:
        raise ReductionError("HueForge filament library has no eligible filaments")
    if not math.isfinite(guidance_strength) or not 0.0 <= guidance_strength <= 1.0:
        raise ReductionError("HueForge guidance strength must be between 0 and 1")
    reduction_options.validate()
    hueforge_options.validate()
    target_lab, target_weights = _stack_target_arrays(target_palette)
    maximum_filaments = min(requested_filaments, len(library.filaments))
    score_cache: dict[tuple[int, ...], float] = {}

    def score(indices: Sequence[int]) -> float:
        key = tuple(sorted(indices))
        cached = score_cache.get(key)
        if cached is not None:
            return cached
        candidates = _generate_hueforge_guidance_candidates(
            key, library, hueforge_options
        )
        value = _guided_candidate_set_score(
            candidates,
            target_lab,
            target_weights,
            hueforge_options.max_perceived_colors,
            reduction_options.min_cluster_fraction,
        )
        score_cache[key] = value
        return value

    first_index = min(
        range(len(library.filaments)), key=lambda index: (score((index,)), index)
    )
    selected = [first_index]
    selected_score = score(selected)
    while len(selected) < maximum_filaments:
        best_addition = -1
        best_score = selected_score
        selected_set = set(selected)
        for candidate_index in range(len(library.filaments)):
            if candidate_index in selected_set:
                continue
            trial_score = score(selected + [candidate_index])
            if trial_score < best_score - 1e-12:
                best_addition = candidate_index
                best_score = trial_score
        if best_addition < 0:
            break
        selected.append(best_addition)
        selected.sort()
        selected_score = best_score

    for _ in range(3):
        best_selection = selected
        best_score = selected_score
        selected_set = set(selected)
        for position in range(len(selected)):
            for replacement in range(len(library.filaments)):
                if replacement in selected_set:
                    continue
                trial = selected.copy()
                trial[position] = replacement
                trial = sorted(trial)
                trial_score = score(trial)
                if trial_score < best_score - 1e-12:
                    best_selection = trial
                    best_score = trial_score
        if best_score >= selected_score - 1e-12:
            break
        selected = best_selection
        selected_score = best_score

    candidates = _generate_hueforge_guidance_candidates(
        selected, library, hueforge_options
    )
    candidate_lab, distance = _guidance_candidate_distance(candidates, target_lab)
    nearest_reference = np.argmin(distance, axis=1)
    guided_lab = (
        (1.0 - guidance_strength) * target_lab
        + guidance_strength * candidate_lab[nearest_reference]
    )
    guided_rgb_values = _lab_to_srgb(guided_lab)

    # Rounding can merge nearby pulled colors. Keep one deterministic provenance
    # record per RGB, preferring the source target with the greater image mass.
    guided_records: dict[tuple[int, int, int], tuple[int, int]] = {}
    for target_index, rgb_value in enumerate(guided_rgb_values):
        rgb = tuple(int(channel) for channel in rgb_value)
        reference_index = int(nearest_reference[target_index])
        existing = guided_records.get(rgb)
        if existing is None or target_weights[target_index] > target_weights[
            existing[0]
        ] + 1e-15:
            guided_records[rgb] = (target_index, reference_index)

    guided_rgbs = tuple(guided_records)
    guided_reference_indices = tuple(
        record[1] for record in guided_records.values()
    )
    guided_candidate_lab = _srgb_to_lab(
        np.asarray(guided_rgbs, dtype=np.uint8)
    ).astype(np.float64)
    local_selected, masses, target_rms = _select_reachable_palette(
        guided_candidate_lab,
        target_lab,
        target_weights,
        hueforge_options.max_perceived_colors,
        reduction_options.min_cluster_fraction,
    )
    records = sorted(
        zip(local_selected, masses),
        key=lambda item: (-float(item[1]), item[0]),
    )
    used_filament_indices = {
        filament_index
        for guided_index, _ in records
        for filament_index in candidates[
            guided_reference_indices[guided_index]
        ].filament_indices
    }
    reported_selected = [
        filament_index
        for filament_index in selected
        if filament_index in used_filament_indices
    ]
    selected_position = {
        filament_index: position
        for position, filament_index in enumerate(reported_selected, start=1)
    }
    total_mass = float(target_weights.sum())
    palette: list[PaletteEntry] = []
    guided_colors: list[HueForgeGuidedColor] = []
    for guided_index, mass in records:
        rgb = guided_rgbs[guided_index]
        reference = candidates[guided_reference_indices[guided_index]]
        lab = guided_candidate_lab[guided_index]
        population = (
            "achromatic"
            if math.hypot(float(lab[1]), float(lab[2]))
            < reduction_options.neutral_chroma
            else "chromatic"
        )
        fraction = float(mass / total_mass) if total_mass else 0.0
        palette.append(
            PaletteEntry(
                rgb=rgb,
                population=population,
                analysis_fraction=fraction,
            )
        )
        guided_colors.append(
            HueForgeGuidedColor(
                rgb=rgb,
                reference_rgb=reference.rgb,
                reference_kind=reference.kind,
                filament_positions=tuple(
                    selected_position[index]
                    for index in reference.filament_indices
                ),
                top_layers=reference.top_layers,
                analysis_fraction=fraction,
            )
        )

    plan = HueForgeGuidancePlan(
        model=HUEFORGE_GUIDANCE_MODEL,
        td_transmission=hueforge_options.td_transmission,
        td_scale=hueforge_options.td_scale,
        layer_height_mm=hueforge_options.layer_height_mm,
        maximum_pairwise_top_layers=hueforge_options.transition_layers,
        guidance_strength=guidance_strength,
        requested_filaments=requested_filaments,
        eligible_filaments=len(library.filaments),
        selected_filaments=tuple(
            library.filaments[index] for index in reported_selected
        ),
        candidate_color_count=len(
            _generate_hueforge_guidance_candidates(
                reported_selected, library, hueforge_options
            )
        ),
        colors=tuple(guided_colors),
        weighted_rms_delta_e76=target_rms,
        library_sha256=library.sha256,
    )
    return tuple(palette), plan


def reduce_image_hueforge_guided(
    source: Image.Image,
    options: ReductionOptions,
    hueforge_options: HueForgeOptions,
    library: HueForgeLibrary,
    guidance_strength: float = DEFAULT_HUEFORGE_GUIDANCE_STRENGTH,
) -> ReductionResult:
    """Prepare an image for HueForge using owned filament colors as anchors."""

    rgba_source = source if source.mode == "RGBA" else source.convert("RGBA")
    analysis_options = replace(
        options, population_colors=hueforge_options.analysis_colors
    )
    target_palette, analysis_size = discover_palette(
        rgba_source, analysis_options
    )
    palette, guidance = _optimize_hueforge_guidance(
        target_palette,
        library,
        options.population_colors,
        options,
        hueforge_options,
        guidance_strength,
    )
    output, quality = _apply_palette_to_pil_image(
        rgba_source, palette, options.chunk_pixels
    )
    digest = hashlib.sha256(output.tobytes()).hexdigest()
    return ReductionResult(
        rgba=output,
        palette=palette,
        source_size=rgba_source.size,
        analysis_size=analysis_size,
        quality=quality,
        rgba_sha256=digest,
        hueforge_guidance=guidance,
    )


def _optimize_hueforge_stack(
    target_palette: Sequence[PaletteEntry],
    library: HueForgeLibrary,
    requested_filaments: int,
    reduction_options: ReductionOptions,
    hueforge_options: HueForgeOptions,
) -> tuple[tuple[PaletteEntry, ...], HueForgeStackPlan]:
    """Select one global filament order and its printable prefix colors."""

    if not target_palette:
        raise ReductionError("HueForge stack optimization needs a target palette")
    if requested_filaments < 1:
        raise ReductionError("HueForge physical filament budget must be positive")
    reduction_options.validate()
    hueforge_options.validate()

    target_lab, target_weights = _stack_target_arrays(target_palette)
    transition_layers = hueforge_options.transition_layers
    maximum_selected_count = min(
        requested_filaments,
        len(library.filaments),
        transition_layers + 1,
    )
    base_candidate_indices = []
    for filament_index, filament in enumerate(library.filaments):
        base_transmission = hueforge_options.td_transmission ** (
            hueforge_options.base_depth_mm
            / (filament.td_mm * hueforge_options.td_scale)
        )
        if base_transmission <= hueforge_options.base_transmission_limit + 1e-12:
            base_candidate_indices.append(filament_index)
    if not base_candidate_indices:
        raise ReductionError(
            "no eligible filament is opaque enough for the requested base depth; "
            "increase --hueforge-base-depth or "
            "--hueforge-base-transmission-limit"
        )

    def make_initial_states() -> list[_StackSearchState]:
        states: list[_StackSearchState] = []
        for filament_index in base_candidate_indices:
            filament = library.filaments[filament_index]
            base_rgb = filament.rgb
            best_distances = _distance_squared_to_color(target_lab, base_rgb)
            states.append(
                _StackSearchState(
                    selected_indices=(filament_index,),
                    run_layers=(hueforge_options.base_layers,),
                    used_transition_layers=0,
                    current_linear=_srgb_to_linear(base_rgb),
                    stack_rgbs=(base_rgb,),
                    stack_layers=(hueforge_options.base_layers,),
                    stack_positions=(1,),
                    best_distances=best_distances,
                    score=float(np.dot(target_weights, best_distances)),
                )
            )
        return states

    state_key = lambda state: (
        state.score,
        state.selected_indices,
        state.run_layers,
    )
    terminal_states: list[_StackSearchState] = []
    for stack_size in range(1, maximum_selected_count + 1):
        beam = sorted(make_initial_states(), key=state_key)[
            : hueforge_options.beam_width
        ]
        if stack_size == 1:
            for state in beam:
                state.score = _stack_state_output_score(
                    state,
                    target_lab,
                    target_weights,
                    hueforge_options.max_perceived_colors,
                    reduction_options.min_cluster_fraction,
                )
            terminal_states.append(min(beam, key=state_key))
            continue

        for position in range(2, stack_size + 1):
            is_final_position = position == stack_size
            remaining_positions = stack_size - position
            expanded: list[_StackSearchState] = []
            for state in beam:
                maximum_run = (
                    transition_layers
                    - state.used_transition_layers
                    - remaining_positions
                )
                if maximum_run < 1:
                    continue
                used = set(state.selected_indices)
                for filament_index, filament in enumerate(library.filaments):
                    if filament_index in used:
                        continue
                    current_linear = state.current_linear.copy()
                    local_rgbs: list[tuple[int, int, int]] = []
                    local_layers: list[int] = []
                    local_positions: list[int] = []
                    best_distances = state.best_distances.copy()
                    for run_length in range(1, maximum_run + 1):
                        current_linear = _simulate_hueforge_layer(
                            current_linear, filament, hueforge_options
                        )
                        rgb_array = _linear_to_srgb(current_linear)
                        rgb = tuple(int(value) for value in rgb_array)
                        local_rgbs.append(rgb)
                        local_layers.append(
                            hueforge_options.base_layers
                            + state.used_transition_layers
                            + run_length
                        )
                        local_positions.append(position)
                        best_distances = np.minimum(
                            best_distances,
                            _distance_squared_to_color(target_lab, rgb),
                        )
                        if is_final_position and run_length != maximum_run:
                            continue
                        expanded.append(
                            _StackSearchState(
                                selected_indices=(
                                    state.selected_indices + (filament_index,)
                                ),
                                run_layers=state.run_layers + (run_length,),
                                used_transition_layers=(
                                    state.used_transition_layers + run_length
                                ),
                                current_linear=current_linear.copy(),
                                stack_rgbs=(
                                    state.stack_rgbs + tuple(local_rgbs)
                                ),
                                stack_layers=(
                                    state.stack_layers + tuple(local_layers)
                                ),
                                stack_positions=(
                                    state.stack_positions + tuple(local_positions)
                                ),
                                best_distances=best_distances.copy(),
                                score=float(
                                    np.dot(target_weights, best_distances)
                                ),
                            )
                        )
            if not expanded:
                raise ReductionError(
                    "could not allocate at least one layer to every selected "
                    "filament"
                )
            if is_final_position:
                for state in expanded:
                    state.score = _stack_state_output_score(
                        state,
                        target_lab,
                        target_weights,
                        hueforge_options.max_perceived_colors,
                        reduction_options.min_cluster_fraction,
                    )
            beam = sorted(expanded, key=state_key)[: hueforge_options.beam_width]
        terminal_states.append(min(beam, key=state_key))

    best_state = min(
        terminal_states,
        key=lambda state: (
            state.score,
            len(state.selected_indices),
            state.selected_indices,
            state.run_layers,
        ),
    )

    candidate_rgbs: list[tuple[int, int, int]] = []
    candidate_layers: list[int] = []
    candidate_positions: list[int] = []
    seen_rgb: set[tuple[int, int, int]] = set()
    for rgb, layer, position in zip(
        best_state.stack_rgbs,
        best_state.stack_layers,
        best_state.stack_positions,
    ):
        if rgb in seen_rgb:
            continue
        seen_rgb.add(rgb)
        candidate_rgbs.append(rgb)
        candidate_layers.append(layer)
        candidate_positions.append(position)

    candidate_lab = _srgb_to_lab(
        np.asarray(candidate_rgbs, dtype=np.uint8)
    ).astype(np.float64)
    selected_candidates, masses, target_rms = _select_reachable_palette(
        candidate_lab,
        target_lab,
        target_weights,
        hueforge_options.max_perceived_colors,
        reduction_options.min_cluster_fraction,
    )
    palette_records = sorted(
        zip(selected_candidates, masses),
        key=lambda item: (candidate_layers[item[0]], item[0]),
    )
    planned_total_layers = max(
        hueforge_options.base_layers,
        max(candidate_layers[index] for index, _ in palette_records),
    )
    total_mass = float(target_weights.sum())
    palette: list[PaletteEntry] = []
    layer_fraction: dict[int, float] = {}
    for candidate_index, mass in palette_records:
        rgb = candidate_rgbs[candidate_index]
        layer = candidate_layers[candidate_index]
        lab = candidate_lab[candidate_index]
        population = (
            "achromatic"
            if math.hypot(float(lab[1]), float(lab[2]))
            < reduction_options.neutral_chroma
            else "chromatic"
        )
        fraction = float(mass / total_mass) if total_mass else 0.0
        palette.append(
            PaletteEntry(
                rgb=rgb,
                population=population,
                analysis_fraction=fraction,
                stack_layer=layer,
                stack_height_mm=round(
                    layer * hueforge_options.layer_height_mm, 10
                ),
                top_filament_position=candidate_positions[candidate_index],
            )
        )
        layer_fraction[layer] = fraction

    runs: list[HueForgeStackRun] = []
    run_start_layer = 1
    for position, (filament_index, run_layers) in enumerate(
        zip(best_state.selected_indices, best_state.run_layers), start=1
    ):
        if run_start_layer > planned_total_layers:
            break
        planned_run_layers = min(
            run_layers, planned_total_layers - run_start_layer + 1
        )
        run_end_layer = run_start_layer + planned_run_layers - 1
        runs.append(
            HueForgeStackRun(
                position=position,
                filament=library.filaments[filament_index],
                layers=planned_run_layers,
                start_layer=run_start_layer,
                end_layer=run_end_layer,
                start_height_mm=round(
                    (run_start_layer - 1) * hueforge_options.layer_height_mm,
                    10,
                ),
                end_height_mm=round(
                    run_end_layer * hueforge_options.layer_height_mm, 10
                ),
            )
        )
        run_start_layer = run_end_layer + 1

    layer_colors: list[HueForgeLayerColor] = []
    base_rgb = library.filaments[best_state.selected_indices[0]].rgb
    for layer in range(1, hueforge_options.base_layers + 1):
        layer_colors.append(
            HueForgeLayerColor(
                layer=layer,
                height_mm=round(layer * hueforge_options.layer_height_mm, 10),
                rgb=base_rgb,
                top_filament_position=1,
                analysis_fraction=layer_fraction.get(layer, 0.0),
            )
        )
    if len(best_state.selected_indices) > 1:
        for rgb, layer, position in zip(
            best_state.stack_rgbs[1:],
            best_state.stack_layers[1:],
            best_state.stack_positions[1:],
        ):
            if layer > planned_total_layers:
                break
            layer_colors.append(
                HueForgeLayerColor(
                    layer=layer,
                    height_mm=round(
                        layer * hueforge_options.layer_height_mm, 10
                    ),
                    rgb=rgb,
                    top_filament_position=position,
                    analysis_fraction=layer_fraction.get(layer, 0.0),
                )
            )

    plan = HueForgeStackPlan(
        model=HUEFORGE_STACK_MODEL,
        td_transmission=hueforge_options.td_transmission,
        td_scale=hueforge_options.td_scale,
        base_transmission_limit=hueforge_options.base_transmission_limit,
        requested_filaments=requested_filaments,
        eligible_filaments=len(library.filaments),
        eligible_base_filaments=len(base_candidate_indices),
        layer_height_mm=hueforge_options.layer_height_mm,
        base_depth_mm=hueforge_options.base_depth_mm,
        max_depth_mm=hueforge_options.max_depth_mm,
        planned_depth_mm=round(
            planned_total_layers * hueforge_options.layer_height_mm, 10
        ),
        runs=tuple(runs),
        layer_colors=tuple(layer_colors),
        weighted_rms_delta_e76=target_rms,
        library_sha256=library.sha256,
    )
    return tuple(palette), plan


def reduce_image_hueforge(
    source: Image.Image,
    options: ReductionOptions,
    hueforge_options: HueForgeOptions,
    library: HueForgeLibrary,
) -> ReductionResult:
    """Reduce an image to colors reachable from one inventory-selected stack."""

    rgba_source = source if source.mode == "RGBA" else source.convert("RGBA")
    analysis_options = replace(
        options, population_colors=hueforge_options.analysis_colors
    )
    target_palette, analysis_size = discover_palette(
        rgba_source, analysis_options
    )
    palette, plan = _optimize_hueforge_stack(
        target_palette,
        library,
        options.population_colors,
        options,
        hueforge_options,
    )
    output, quality = _apply_palette_to_pil_image(
        rgba_source, palette, options.chunk_pixels
    )
    stack_layer_map = np.zeros(output.shape[:2], dtype=np.uint16)
    visible = output[..., 3] > 0
    for entry in palette:
        if entry.stack_layer is None:
            continue
        matches = np.all(
            output[..., :3] == np.asarray(entry.rgb, dtype=np.uint8), axis=2
        )
        stack_layer_map[matches & visible] = entry.stack_layer
    digest = hashlib.sha256(output.tobytes()).hexdigest()
    return ReductionResult(
        rgba=output,
        palette=palette,
        source_size=rgba_source.size,
        analysis_size=analysis_size,
        quality=quality,
        rgba_sha256=digest,
        hueforge_plan=plan,
        stack_layer_map=stack_layer_map,
    )


def apply_palette_full_resolution(
    source_rgba: np.ndarray,
    palette: Sequence[PaletteEntry],
    chunk_pixels: int,
) -> tuple[np.ndarray, QualityMetrics]:
    """Map every source pixel to the nearest palette color in bounded chunks."""

    source = np.asarray(source_rgba, dtype=np.uint8)
    if source.ndim != 3 or source.shape[2] != 4:
        raise ReductionError("source pixel array must have shape (height, width, 4)")
    if not palette:
        raise ReductionError("palette cannot be empty")

    palette_rgb = np.asarray([entry.rgb for entry in palette], dtype=np.uint8)
    palette_lab = _srgb_to_lab(palette_rgb).astype(np.float32)
    flat_source = source.reshape(-1, 4)
    output = np.empty_like(flat_source)
    output[:, 3] = flat_source[:, 3]

    weighted_delta_sum = 0.0
    weighted_delta_squared_sum = 0.0
    visible_weight_sum = 0.0
    maximum_delta = 0.0

    for start in range(0, flat_source.shape[0], chunk_pixels):
        stop = min(start + chunk_pixels, flat_source.shape[0])
        source_chunk = flat_source[start:stop]
        lab_chunk = _srgb_to_lab(source_chunk[:, :3])
        best_distance = np.full(lab_chunk.shape[0], np.inf, dtype=np.float32)
        best_index = np.zeros(lab_chunk.shape[0], dtype=np.int32)

        for palette_index, center in enumerate(palette_lab):
            difference = lab_chunk - center
            distance = np.einsum(
                "ij,ij->i", difference, difference, optimize=True
            )
            closer = distance < best_distance
            best_distance[closer] = distance[closer]
            best_index[closer] = palette_index

        output[start:stop, :3] = palette_rgb[best_index]
        alpha_weight = source_chunk[:, 3].astype(np.float64) / 255.0
        visible = alpha_weight > 0
        if np.any(visible):
            delta = np.sqrt(best_distance[visible].astype(np.float64))
            weights = alpha_weight[visible]
            weighted_delta_sum += float(np.sum(delta * weights))
            weighted_delta_squared_sum += float(np.sum(delta * delta * weights))
            visible_weight_sum += float(np.sum(weights))
            maximum_delta = max(maximum_delta, float(np.max(delta)))

    # Hidden RGB must not create an extra nominal color in the saved PNG. Using
    # the first real palette entry keeps every output RGB inside the palette.
    transparent = flat_source[:, 3] == 0
    output[transparent, :3] = palette_rgb[0]
    if visible_weight_sum:
        mean_delta = weighted_delta_sum / visible_weight_sum
        rms_delta = math.sqrt(weighted_delta_squared_sum / visible_weight_sum)
    else:
        mean_delta = 0.0
        rms_delta = 0.0
    metrics = QualityMetrics(mean_delta, rms_delta, maximum_delta)
    return output.reshape(source.shape), metrics


def _apply_palette_to_pil_image(
    source: Image.Image,
    palette: Sequence[PaletteEntry],
    chunk_pixels: int,
) -> tuple[np.ndarray, QualityMetrics]:
    """Map PIL row bands without materializing a second full source array."""

    if source.mode != "RGBA":
        raise ReductionError("PIL source must be normalized to RGBA")
    if not palette:
        raise ReductionError("palette cannot be empty")

    width, height = source.size
    rows_per_chunk = max(1, chunk_pixels // width)
    output = np.empty((height, width, 4), dtype=np.uint8)
    palette_rgb = np.asarray([entry.rgb for entry in palette], dtype=np.uint8)
    palette_lab = _srgb_to_lab(palette_rgb).astype(np.float32)
    weighted_delta_sum = 0.0
    weighted_delta_squared_sum = 0.0
    visible_weight_sum = 0.0
    maximum_delta = 0.0

    for top in range(0, height, rows_per_chunk):
        bottom = min(top + rows_per_chunk, height)
        source_chunk = np.asarray(
            source.crop((0, top, width, bottom)), dtype=np.uint8
        ).reshape(-1, 4)
        lab_chunk = _srgb_to_lab(source_chunk[:, :3])
        best_distance = np.full(lab_chunk.shape[0], np.inf, dtype=np.float32)
        best_index = np.zeros(lab_chunk.shape[0], dtype=np.int32)
        for palette_index, center in enumerate(palette_lab):
            difference = lab_chunk - center
            distance = np.einsum(
                "ij,ij->i", difference, difference, optimize=True
            )
            closer = distance < best_distance
            best_distance[closer] = distance[closer]
            best_index[closer] = palette_index

        mapped = np.empty_like(source_chunk)
        mapped[:, :3] = palette_rgb[best_index]
        mapped[:, 3] = source_chunk[:, 3]
        transparent = source_chunk[:, 3] == 0
        mapped[transparent, :3] = palette_rgb[0]
        output[top:bottom] = mapped.reshape(bottom - top, width, 4)

        alpha_weight = source_chunk[:, 3].astype(np.float64) / 255.0
        visible = alpha_weight > 0
        if np.any(visible):
            delta = np.sqrt(best_distance[visible].astype(np.float64))
            weights = alpha_weight[visible]
            weighted_delta_sum += float(np.sum(delta * weights))
            weighted_delta_squared_sum += float(np.sum(delta * delta * weights))
            visible_weight_sum += float(np.sum(weights))
            maximum_delta = max(maximum_delta, float(np.max(delta)))

    if visible_weight_sum:
        mean_delta = weighted_delta_sum / visible_weight_sum
        rms_delta = math.sqrt(weighted_delta_squared_sum / visible_weight_sum)
    else:
        mean_delta = 0.0
        rms_delta = 0.0
    return output, QualityMetrics(mean_delta, rms_delta, maximum_delta)


def reduce_image(source: Image.Image, options: ReductionOptions) -> ReductionResult:
    """Discover a palette and apply it to the full-resolution RGBA source."""

    rgba_source = source if source.mode == "RGBA" else source.convert("RGBA")
    palette, analysis_size = discover_palette(rgba_source, options)
    output, quality = _apply_palette_to_pil_image(
        rgba_source, palette, options.chunk_pixels
    )
    digest = hashlib.sha256(output.tobytes()).hexdigest()
    return ReductionResult(
        rgba=output,
        palette=palette,
        source_size=rgba_source.size,
        analysis_size=analysis_size,
        quality=quality,
        rgba_sha256=digest,
    )


def save_result(
    result: ReductionResult,
    destination: Path,
    *,
    had_alpha: bool,
    metadata: dict[str, object] | None = None,
    overwrite: bool = False,
) -> None:
    """Save a lossless PNG without silently replacing an existing file."""

    if destination.exists() and not overwrite:
        raise ReductionError(
            f"output already exists: {destination} (pass --force to replace it)"
        )
    if destination.suffix.lower() != ".png":
        raise ReductionError("output must use the .png extension")
    temporary_path: Path | None = None
    try:
        destination.parent.mkdir(parents=True, exist_ok=True)
        output = Image.fromarray(result.rgba, mode="RGBA")
        if not had_alpha:
            output = output.convert("RGB")
        descriptor, temporary_name = tempfile.mkstemp(
            dir=destination.parent,
            prefix=f".{destination.name}.",
            suffix=".tmp",
        )
        temporary_path = Path(temporary_name)
        os.close(descriptor)
        output.save(
            temporary_path,
            format="PNG",
            optimize=False,
            compress_level=9,
            **(metadata or {}),
        )
        _commit_temporary_file(
            temporary_path,
            destination,
            overwrite=overwrite,
            existing_label="output",
        )
        temporary_path = None
    except OSError as error:
        raise ReductionError(f"could not write {destination}: {error}") from error
    finally:
        if temporary_path is not None:
            temporary_path.unlink(missing_ok=True)


def save_stack_layer_map(
    result: ReductionResult,
    destination: Path,
    *,
    overwrite: bool = False,
) -> None:
    """Save exact 1-based stack-layer assignments as a 16-bit PNG."""

    if result.stack_layer_map is None or result.hueforge_plan is None:
        raise ReductionError("a stack layer map is only available in HueForge mode")
    if destination.exists() and not overwrite:
        raise ReductionError(
            f"stack layer map already exists: {destination} "
            "(pass --force to replace it)"
        )
    if destination.suffix.lower() != ".png":
        raise ReductionError("stack layer map must use the .png extension")
    layer_map = np.asarray(result.stack_layer_map)
    if layer_map.ndim != 2 or layer_map.shape != result.rgba.shape[:2]:
        raise ReductionError("stack layer map dimensions do not match the output")
    if np.any(layer_map < 0) or np.any(layer_map > np.iinfo(np.uint16).max):
        raise ReductionError("stack layer map values do not fit a 16-bit PNG")

    temporary_path: Path | None = None
    try:
        destination.parent.mkdir(parents=True, exist_ok=True)
        descriptor, temporary_name = tempfile.mkstemp(
            dir=destination.parent,
            prefix=f".{destination.name}.",
            suffix=".tmp",
        )
        temporary_path = Path(temporary_name)
        os.close(descriptor)
        Image.fromarray(layer_map.astype(np.uint16, copy=False)).save(
            temporary_path,
            format="PNG",
            optimize=False,
            compress_level=9,
        )
        _commit_temporary_file(
            temporary_path,
            destination,
            overwrite=overwrite,
            existing_label="stack layer map",
        )
        temporary_path = None
    except OSError as error:
        raise ReductionError(
            f"could not write stack layer map {destination}: {error}"
        ) from error
    finally:
        if temporary_path is not None:
            temporary_path.unlink(missing_ok=True)


def _commit_temporary_file(
    temporary_path: Path,
    destination: Path,
    *,
    overwrite: bool,
    existing_label: str,
) -> None:
    """Publish a complete temp file without a check-then-replace race."""

    if overwrite:
        os.replace(temporary_path, destination)
        return
    try:
        # The temp file is in destination.parent. A hard link publishes the
        # complete bytes atomically and fails if the destination appeared while
        # those bytes were being encoded.
        os.link(temporary_path, destination)
    except FileExistsError as error:
        raise ReductionError(
            f"{existing_label} already exists: {destination} "
            "(pass --force to replace it)"
        ) from error
    except OSError as error:
        raise ReductionError(
            f"could not atomically create {destination}: {error}; "
            "use a local filesystem that supports hard links or pass --force"
        ) from error
    temporary_path.unlink()


def write_palette_json(
    result: ReductionResult,
    destination: Path,
    *,
    overwrite: bool = False,
) -> None:
    if destination.exists() and not overwrite:
        raise ReductionError(
            f"palette report already exists: {destination} "
            "(pass --force to replace it)"
        )
    palette_payload: list[dict[str, object]] = []
    for entry in result.palette:
        record: dict[str, object] = {
            "rgb": list(entry.rgb),
            "hex": entry.hex_color,
            "population": entry.population,
            "analysis_fraction": entry.analysis_fraction,
        }
        if entry.stack_layer is not None:
            record["stack_layer"] = entry.stack_layer
        if entry.stack_height_mm is not None:
            record["stack_height_mm"] = entry.stack_height_mm
        if entry.top_filament_position is not None:
            record["top_filament_position"] = entry.top_filament_position
        palette_payload.append(record)

    if result.hueforge_plan is not None:
        report_mode = "hueforge-stack"
    elif result.hueforge_guidance is not None:
        report_mode = "hueforge-library-guided"
    else:
        report_mode = "perceptual"
    payload = {
        "mode": report_mode,
        "source_size": list(result.source_size),
        "analysis_size": list(result.analysis_size),
        "rgba_sha256": result.rgba_sha256,
        "quality": {
            "mean_delta_e76": result.quality.mean_delta_e76,
            "rms_delta_e76": result.quality.rms_delta_e76,
            "max_delta_e76": result.quality.max_delta_e76,
        },
        "palette": palette_payload,
    }
    if result.hueforge_guidance is not None:
        guidance = result.hueforge_guidance
        payload["hueforge_guidance"] = {
            "model": guidance.model,
            "intended_use": (
                "Library-guided image preprocessing for subsequent import into "
                "HueForge; this is not a global swap or height plan."
            ),
            "model_note": (
                "Output colors are pulled toward selected owned filament RGB "
                "values and independent approximate TD-aware ordered-pair "
                "hues. HueForge performs the final stack conversion."
            ),
            "td_transmission": guidance.td_transmission,
            "front_lit_td_scale": guidance.td_scale,
            "layer_height_mm": guidance.layer_height_mm,
            "maximum_pairwise_top_layers": (
                guidance.maximum_pairwise_top_layers
            ),
            "guidance_strength": guidance.guidance_strength,
            "global_stack_guaranteed": False,
            "requested_filament_anchors": guidance.requested_filaments,
            "eligible_filaments": guidance.eligible_filaments,
            "selected_filament_count": len(guidance.selected_filaments),
            "candidate_color_count": guidance.candidate_color_count,
            "output_palette_count": len(result.palette),
            "weighted_rms_target_delta_e76": (
                guidance.weighted_rms_delta_e76
            ),
            "library_sha256": guidance.library_sha256,
            "search_method": guidance.search_method,
            "selected_filaments": [
                {
                    "position": position,
                    "brand": filament.brand,
                    "name": filament.name,
                    "rgb": list(filament.rgb),
                    "hex": filament.hex_color,
                    "td_mm": filament.td_mm,
                    "type": filament.material_type,
                    "uuid": filament.uuid,
                    "owned": filament.owned,
                    "tags": list(filament.tags),
                    "source_index": filament.source_index,
                    "secondary_color_approximated_as_primary": (
                        filament.has_secondary_color
                    ),
                }
                for position, filament in enumerate(
                    guidance.selected_filaments, start=1
                )
            ],
            "guided_palette_sources": [
                {
                    "rgb": list(color.rgb),
                    "hex": color.hex_color,
                    "reference_rgb": list(color.reference_rgb),
                    "reference_hex": "#{:02X}{:02X}{:02X}".format(
                        *color.reference_rgb
                    ),
                    "reference_kind": color.reference_kind,
                    "filament_positions": list(color.filament_positions),
                    "top_layers": color.top_layers,
                    "top_thickness_mm": round(
                        color.top_layers * guidance.layer_height_mm, 10
                    ),
                    "analysis_fraction": color.analysis_fraction,
                }
                for color in guidance.colors
            ],
        }
    if result.hueforge_plan is not None:
        plan = result.hueforge_plan
        payload["hueforge_stack"] = {
            "model": plan.model,
            "model_note": (
                "Independent ColorNinja approximation; predicted colors are not "
                "guaranteed to match HueForge or a physical print."
            ),
            "transmission_formula": (
                "effective_td_mm = library_td_mm * td_scale; "
                "T = td_transmission ** (thickness_mm / effective_td_mm)"
            ),
            "compositing": (
                "linear sRGB, bottom to top: output = "
                "(1 - T) * top + T * below"
            ),
            "td_transmission": plan.td_transmission,
            "front_lit_td_scale": plan.td_scale,
            "base_transmission_limit": plan.base_transmission_limit,
            "requested_filaments": plan.requested_filaments,
            "eligible_filaments": plan.eligible_filaments,
            "eligible_base_filaments": plan.eligible_base_filaments,
            "selected_filament_count": len(plan.runs),
            "perceived_palette_count": len(result.palette),
            "layer_index_base": 1,
            "layer_height_mm": plan.layer_height_mm,
            "first_layer_height_assumption_mm": plan.layer_height_mm,
            "base_depth_mm": plan.base_depth_mm,
            "blend_depth_above_base_mm": round(
                plan.planned_depth_mm - plan.base_depth_mm, 10
            ),
            "planned_total_height_mm": plan.planned_depth_mm,
            "actual_used_height_mm": plan.planned_depth_mm,
            "maximum_allowed_total_height_mm": plan.max_depth_mm,
            "height_note": (
                "all layers, including the first, are assumed to use layer_height_mm; "
                "planned_total_height_mm is the final Z height; HueForge versions "
                "that label Blend Depth as depth above the base should use "
                "blend_depth_above_base_mm"
            ),
            "weighted_rms_target_delta_e76": plan.weighted_rms_delta_e76,
            "library_sha256": plan.library_sha256,
            "search_method": plan.search_method,
            "runs_bottom_to_top": [
                {
                    "position": run.position,
                    "layers": run.layers,
                    "start_layer_inclusive": run.start_layer,
                    "end_layer_inclusive": run.end_layer,
                    "start_height_mm": run.start_height_mm,
                    "end_height_mm": run.end_height_mm,
                    "swap_at_height_mm": (
                        None if run.position == 1 else run.start_height_mm
                    ),
                    "swap_before_layer": (
                        None if run.position == 1 else run.start_layer
                    ),
                    "filament": {
                        "brand": run.filament.brand,
                        "name": run.filament.name,
                        "rgb": list(run.filament.rgb),
                        "hex": run.filament.hex_color,
                        "td_mm": run.filament.td_mm,
                        "type": run.filament.material_type,
                        "uuid": run.filament.uuid,
                        "owned": run.filament.owned,
                        "tags": list(run.filament.tags),
                        "source_index": run.filament.source_index,
                        "secondary_color_approximated_as_primary": (
                            run.filament.has_secondary_color
                        ),
                    },
                }
                for run in plan.runs
            ],
            "layer_colors": [
                {
                    "layer": layer.layer,
                    "height_mm": layer.height_mm,
                    "rgb": list(layer.rgb),
                    "hex": "#{:02X}{:02X}{:02X}".format(*layer.rgb),
                    "top_filament_position": layer.top_filament_position,
                    "used_by_output": layer.analysis_fraction > 0,
                    "analysis_fraction": layer.analysis_fraction,
                }
                for layer in plan.layer_colors
            ],
        }
        if result.stack_layer_map is not None:
            layer_bytes = result.stack_layer_map.astype(
                "<u2", copy=False
            ).tobytes()
            payload["hueforge_stack"]["layer_index_map"] = {
                "encoding": "16-bit unsigned PNG values are exact 1-based stack layers",
                "transparent_pixel_value": 0,
                "visible_minimum_value": 1,
                "maximum_planned_value": max(
                    layer.layer for layer in plan.layer_colors
                ),
                "raw_little_endian_u16_sha256": hashlib.sha256(
                    layer_bytes
                ).hexdigest(),
                "write_with": "--hueforge-height-map PATH.png",
            }
    temporary_path: Path | None = None
    try:
        destination.parent.mkdir(parents=True, exist_ok=True)
        descriptor, temporary_name = tempfile.mkstemp(
            dir=destination.parent,
            prefix=f".{destination.name}.",
            suffix=".tmp",
        )
        temporary_path = Path(temporary_name)
        with os.fdopen(
            descriptor, "w", encoding="utf-8", errors="strict", newline="\n"
        ) as output:
            output.write(json.dumps(payload, indent=2) + "\n")
        _commit_temporary_file(
            temporary_path,
            destination,
            overwrite=overwrite,
            existing_label="palette report",
        )
        temporary_path = None
    except OSError as error:
        raise ReductionError(
            f"could not write palette report {destination}: {error}"
        ) from error
    finally:
        if temporary_path is not None:
            temporary_path.unlink(missing_ok=True)


def _default_output_path(source: Path) -> Path:
    return source.with_name(f"{source.stem}-reduced.png")


def _build_argument_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description=(
            "Reduce an image to a deterministic perceptual palette and save a "
            "lossless, full-resolution PNG."
        ),
        epilog=(
            "Normally --colors is a ceiling for each color population. With "
            "--hueforge-library it is an upper bound on owned filament anchors. "
            "Add --hueforge-stack only when an explicit global stack plan is wanted."
        ),
    )
    parser.add_argument("input", type=Path, help="local source image")
    parser.add_argument(
        "-o",
        "--output",
        type=Path,
        help="output PNG (default: <input>-reduced.png)",
    )
    parser.add_argument(
        "--colors",
        type=int,
        default=None,
        metavar="N",
        help=(
            "cluster ceiling per population, or owned filament-anchor budget "
            "with a HueForge library (defaults: 32 classic, 4 HueForge)"
        ),
    )
    parser.add_argument(
        "--analysis-max-pixels",
        type=int,
        default=DEFAULT_ANALYSIS_MAX_PIXELS,
        metavar="N",
        help="palette-analysis pixel ceiling; 0 uses every source pixel (default: 6291456)",
    )
    parser.add_argument(
        "--full-analysis",
        action="store_true",
        help="learn the palette from every source pixel (same as --analysis-max-pixels 0)",
    )
    parser.add_argument(
        "--neutral-chroma",
        type=float,
        default=8.0,
        metavar="C",
        help="CIE Lab chroma below which colors are achromatic (default: 8.0)",
    )
    parser.add_argument(
        "--min-cluster-percent",
        type=float,
        default=0.5,
        metavar="PCT",
        help="cull clusters below this share of their population (default: 0.5)",
    )
    parser.add_argument(
        "--histogram-bits",
        type=int,
        default=6,
        metavar="N",
        help="RGB histogram precision per channel, 3-7 (default: 6)",
    )
    parser.add_argument(
        "--iterations",
        type=int,
        default=24,
        metavar="N",
        help="maximum clustering iterations (default: 24)",
    )
    parser.add_argument(
        "--chunk-pixels",
        type=int,
        default=DEFAULT_CHUNK_PIXELS,
        metavar="N",
        help="full-resolution mapping chunk size (default: 131072)",
    )
    parser.add_argument(
        "--preblur-sigma",
        type=float,
        default=1.5,
        metavar="PX",
        help="palette-discovery Gaussian sigma; 0 disables it (default: 1.5)",
    )
    parser.add_argument(
        "--palette-json",
        type=Path,
        help="optional JSON palette and quality report",
    )
    parser.add_argument(
        "--hueforge-library",
        "--filament-library",
        dest="hueforge_library",
        type=Path,
        metavar="PATH",
        help=(
            "exported HueForge filament-library JSON; guides reduction toward "
            "owned filament colors and their approximate perceived hues"
        ),
    )
    parser.add_argument(
        "--hueforge-stack",
        action="store_true",
        help=(
            "advanced: produce one explicit global stack plan instead of the "
            "default library-guided preprocessing mode"
        ),
    )
    parser.add_argument(
        "--hueforge-height-map",
        type=Path,
        metavar="PATH",
        help=(
            "strict stack mode only: optional 16-bit PNG whose visible pixel "
            "values are exact 1-based stack layer assignments"
        ),
    )
    parser.add_argument(
        "--hueforge-include-unowned",
        action="store_true",
        help="allow library entries whose Owned field is false",
    )
    parser.add_argument(
        "--hueforge-type",
        action="append",
        default=[],
        metavar="TYPE",
        help="only use this material Type; repeat to allow multiple types",
    )
    parser.add_argument(
        "--hueforge-allow-secondary",
        action="store_true",
        help=(
            "include dual/secondary-color entries using only their primary Color "
            "as an approximation"
        ),
    )
    parser.add_argument(
        "--hueforge-layer-height",
        type=float,
        default=DEFAULT_HUEFORGE_LAYER_HEIGHT_MM,
        metavar="MM",
        help="uniform printed layer height in mm (default: 0.08)",
    )
    parser.add_argument(
        "--hueforge-base-depth",
        type=float,
        default=DEFAULT_HUEFORGE_BASE_DEPTH_MM,
        metavar="MM",
        help="minimum opaque base thickness in mm (default: 0.48)",
    )
    parser.add_argument(
        "--hueforge-max-depth",
        type=float,
        default=DEFAULT_HUEFORGE_MAX_DEPTH_MM,
        metavar="MM",
        help="maximum allowed total printed height in mm (default: 2.24)",
    )
    parser.add_argument(
        "--hueforge-analysis-colors",
        type=int,
        default=DEFAULT_HUEFORGE_ANALYSIS_COLORS,
        metavar="N",
        help=(
            "target clusters per image population during library guidance or "
            "stack search (default: 32)"
        ),
    )
    parser.add_argument(
        "--hueforge-beam-width",
        type=int,
        default=DEFAULT_HUEFORGE_BEAM_WIDTH,
        metavar="N",
        help="number of partial global stacks retained during search (default: 24)",
    )
    parser.add_argument(
        "--hueforge-max-perceived-colors",
        type=int,
        default=DEFAULT_HUEFORGE_MAX_PERCEIVED_COLORS,
        metavar="N",
        help="maximum guided/reachable hues in the output PNG (default: 64)",
    )
    parser.add_argument(
        "--hueforge-guidance-strength",
        type=float,
        default=DEFAULT_HUEFORGE_GUIDANCE_STRENGTH,
        metavar="F",
        help=(
            "guided mode: 0 keeps analyzed colors and 1 fully snaps to "
            "owned-derived hues (default: 0.8)"
        ),
    )
    parser.add_argument(
        "--hueforge-td-transmission",
        type=float,
        default=DEFAULT_HUEFORGE_TD_TRANSMISSION,
        metavar="F",
        help=(
            "modeled transmitted fraction at one effective front-lit TD, "
            "between 0 and 1 (default: 0.05)"
        ),
    )
    parser.add_argument(
        "--hueforge-td-scale",
        type=float,
        default=DEFAULT_HUEFORGE_TD_SCALE,
        metavar="F",
        help=(
            "multiply stored back-lit TD by this approximate front-lit scale "
            "(default: 0.1)"
        ),
    )
    parser.add_argument(
        "--hueforge-base-transmission-limit",
        type=float,
        default=DEFAULT_HUEFORGE_BASE_TRANSMISSION_LIMIT,
        metavar="F",
        help=(
            "maximum modeled transmission allowed for a base filament "
            "(default: 0.10)"
        ),
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="replace an existing output file",
    )
    parser.add_argument(
        "--quiet",
        action="store_true",
        help="suppress the processing summary",
    )
    return parser


def main(argv: Optional[Sequence[str]] = None) -> int:
    arguments = _build_argument_parser().parse_args(argv)
    hueforge_mode = arguments.hueforge_library is not None
    color_budget = arguments.colors
    if color_budget is None:
        color_budget = (
            DEFAULT_HUEFORGE_FILAMENTS
            if hueforge_mode
            else DEFAULT_POPULATION_COLORS
        )
    if not hueforge_mode and (
        arguments.hueforge_include_unowned
        or arguments.hueforge_type
        or arguments.hueforge_allow_secondary
        or arguments.hueforge_height_map
        or arguments.hueforge_stack
    ):
        print(
            "error: HueForge-specific options require --hueforge-library",
            file=sys.stderr,
        )
        return 1
    if arguments.hueforge_height_map and not arguments.hueforge_stack:
        print(
            "error: --hueforge-height-map requires --hueforge-stack",
            file=sys.stderr,
        )
        return 1

    output_path = arguments.output or _default_output_path(arguments.input)
    input_resolved = arguments.input.resolve()
    output_resolved = output_path.resolve()
    if input_resolved == output_resolved:
        print("error: input and output paths must be different", file=sys.stderr)
        return 1
    if output_path.suffix.lower() != ".png":
        print("error: output must use the .png extension", file=sys.stderr)
        return 1
    if output_path.exists() and not arguments.force:
        print(
            f"error: output already exists: {output_path} "
            "(pass --force to replace it)",
            file=sys.stderr,
        )
        return 1
    if arguments.palette_json:
        palette_resolved = arguments.palette_json.resolve()
        if palette_resolved in (input_resolved, output_resolved):
            print(
                "error: palette JSON path must differ from input and output paths",
                file=sys.stderr,
            )
            return 1
        if arguments.palette_json.exists() and not arguments.force:
            print(
                f"error: palette report already exists: {arguments.palette_json} "
                "(pass --force to replace it)",
                file=sys.stderr,
            )
            return 1
    if arguments.hueforge_height_map:
        height_map_resolved = arguments.hueforge_height_map.resolve()
        height_conflicts = {input_resolved, output_resolved}
        if arguments.palette_json:
            height_conflicts.add(arguments.palette_json.resolve())
        if height_map_resolved in height_conflicts:
            print(
                "error: HueForge height-map path must differ from input, output, "
                "and palette report paths",
                file=sys.stderr,
            )
            return 1
        if arguments.hueforge_height_map.suffix.lower() != ".png":
            print(
                "error: HueForge height map must use the .png extension",
                file=sys.stderr,
            )
            return 1
        if arguments.hueforge_height_map.exists() and not arguments.force:
            print(
                f"error: stack layer map already exists: "
                f"{arguments.hueforge_height_map} (pass --force to replace it)",
                file=sys.stderr,
            )
            return 1
    if arguments.hueforge_library:
        library_resolved = arguments.hueforge_library.resolve()
        conflicting_paths = {input_resolved, output_resolved}
        if arguments.palette_json:
            conflicting_paths.add(arguments.palette_json.resolve())
        if arguments.hueforge_height_map:
            conflicting_paths.add(arguments.hueforge_height_map.resolve())
        if library_resolved in conflicting_paths:
            print(
                "error: HueForge library path must differ from input, output, "
                "palette report, and height-map paths",
                file=sys.stderr,
            )
            return 1

    analysis_max_pixels = (
        0 if arguments.full_analysis else arguments.analysis_max_pixels
    )
    options = ReductionOptions(
        population_colors=color_budget,
        analysis_max_pixels=analysis_max_pixels,
        neutral_chroma=arguments.neutral_chroma,
        min_cluster_fraction=arguments.min_cluster_percent / 100.0,
        histogram_bits=arguments.histogram_bits,
        max_iterations=arguments.iterations,
        chunk_pixels=arguments.chunk_pixels,
        preblur_sigma=arguments.preblur_sigma,
    )

    started = time.perf_counter()
    library: HueForgeLibrary | None = None
    try:
        options.validate()
        hueforge_options: HueForgeOptions | None = None
        if arguments.hueforge_library:
            hueforge_options = HueForgeOptions(
                layer_height_mm=arguments.hueforge_layer_height,
                base_depth_mm=arguments.hueforge_base_depth,
                max_depth_mm=arguments.hueforge_max_depth,
                analysis_colors=arguments.hueforge_analysis_colors,
                beam_width=arguments.hueforge_beam_width,
                max_perceived_colors=arguments.hueforge_max_perceived_colors,
                td_transmission=arguments.hueforge_td_transmission,
                td_scale=arguments.hueforge_td_scale,
                base_transmission_limit=(
                    arguments.hueforge_base_transmission_limit
                ),
            )
            hueforge_options.validate()
            library = load_hueforge_library(
                arguments.hueforge_library,
                include_unowned=arguments.hueforge_include_unowned,
                material_types=arguments.hueforge_type,
                allow_secondary=arguments.hueforge_allow_secondary,
            )
            if library.skipped_invalid:
                print(
                    "warning: skipped "
                    f"{library.skipped_invalid} invalid filament library "
                    "entr"
                    f"{'y' if library.skipped_invalid == 1 else 'ies'}",
                    file=sys.stderr,
                )
            if library.skipped_secondary and not arguments.hueforge_allow_secondary:
                print(
                    "warning: skipped "
                    f"{library.skipped_secondary} secondary-color filament "
                    "entr"
                    f"{'y' if library.skipped_secondary == 1 else 'ies'}; "
                    "pass --hueforge-allow-secondary to approximate them by "
                    "their primary Color",
                    file=sys.stderr,
                )
            if arguments.hueforge_allow_secondary and any(
                filament.has_secondary_color for filament in library.filaments
            ):
                print(
                    "warning: secondary-color filaments are being approximated "
                    "using only their primary Color",
                    file=sys.stderr,
                )
        loaded = load_image(arguments.input)
        if library is not None and hueforge_options is not None:
            if arguments.hueforge_stack:
                result = reduce_image_hueforge(
                    loaded.rgba, options, hueforge_options, library
                )
                selected_for_material_check = tuple(
                    run.filament for run in result.hueforge_plan.runs
                )
            else:
                result = reduce_image_hueforge_guided(
                    loaded.rgba,
                    options,
                    hueforge_options,
                    library,
                    arguments.hueforge_guidance_strength,
                )
                selected_for_material_check = (
                    result.hueforge_guidance.selected_filaments
                )
            material_families = {
                re.split(r"[\s_-]+", filament.material_type.strip(), maxsplit=1)[
                    0
                ].casefold()
                for filament in selected_for_material_check
                if filament.material_type.strip()
            }
            if len(material_families) > 1:
                print(
                    "warning: selected filaments mix material families "
                    f"({', '.join(sorted(material_families))}); verify adhesion "
                    "and temperature compatibility or filter with "
                    "--hueforge-type",
                    file=sys.stderr,
                )
        else:
            result = reduce_image(loaded.rgba, options)
        had_alpha = loaded.had_alpha
        save_metadata = loaded.save_metadata
        del loaded
        save_result(
            result,
            output_path,
            had_alpha=had_alpha,
            metadata=save_metadata,
            overwrite=arguments.force,
        )
        if arguments.hueforge_height_map:
            save_stack_layer_map(
                result,
                arguments.hueforge_height_map,
                overwrite=arguments.force,
            )
        if arguments.palette_json:
            write_palette_json(
                result, arguments.palette_json, overwrite=arguments.force
            )
    except ReductionError as error:
        print(f"error: {error}", file=sys.stderr)
        return 1
    except MemoryError:
        print(
            "error: the image is too large for the available memory; "
            "lower --analysis-max-pixels or use a smaller source",
            file=sys.stderr,
        )
        return 1

    if not arguments.quiet:
        elapsed = time.perf_counter() - started
        chromatic = sum(
            entry.population == "chromatic" for entry in result.palette
        )
        achromatic = len(result.palette) - chromatic
        print(f"Source:   {result.source_size[0]}x{result.source_size[1]}")
        print(f"Analysis: {result.analysis_size[0]}x{result.analysis_size[1]}")
        if result.hueforge_plan is None and result.hueforge_guidance is None:
            print(
                f"Palette:  {len(result.palette)} colors "
                f"({chromatic} chromatic, {achromatic} achromatic)"
            )
        elif result.hueforge_guidance is not None:
            guidance = result.hueforge_guidance
            print(
                f"Filament anchors: {len(guidance.selected_filaments)} selected "
                f"of {guidance.eligible_filaments} eligible (requested "
                f"{guidance.requested_filaments})"
            )
            for position, filament in enumerate(
                guidance.selected_filaments, start=1
            ):
                print(
                    f"  {position}. {filament.hex_color} "
                    f"{filament.brand} {filament.name} "
                    f"(TD {filament.td_mm:g} mm, {filament.material_type})"
                )
            print(
                f"Guided palette: {len(result.palette)} colors pulled "
                f"{guidance.guidance_strength:.0%} toward owned/pairwise "
                "reference hues "
                f"({chromatic} chromatic, {achromatic} achromatic)"
            )
            print("Next:      Open the saved PNG in HueForge for final conversion")
            if library is not None:
                print(
                    "Library:   "
                    f"{library.total_entries} entries, "
                    f"{len(library.filaments)} eligible, "
                    f"{library.skipped_unowned} unowned skipped, "
                    f"{library.skipped_filtered} type-filtered, "
                    f"{library.skipped_duplicate} optical duplicates skipped"
                )
        else:
            plan = result.hueforge_plan
            print(
                f"Filaments: {len(plan.runs)} selected of "
                f"{plan.eligible_filaments} eligible (requested "
                f"{plan.requested_filaments})"
            )
            print(
                f"Stack:     {plan.layer_height_mm:g} mm layers, "
                f"{plan.base_depth_mm:g} mm base, "
                f"{plan.planned_depth_mm:g} mm planned total "
                f"({plan.planned_depth_mm - plan.base_depth_mm:g} mm above base; "
                f"{plan.max_depth_mm:g} mm ceiling)"
            )
            for run in plan.runs:
                filament = run.filament
                swap = (
                    "base"
                    if run.position == 1
                    else f"swap at {run.start_height_mm:.2f} mm"
                )
                print(
                    f"  {run.position}. {filament.hex_color} "
                    f"{filament.brand} {filament.name} "
                    f"(TD {filament.td_mm:g} mm, {run.layers} layers, "
                    f"{swap}; through {run.end_height_mm:.2f} mm)"
                )
            print(
                f"Perceived palette: {len(result.palette)} reachable colors "
                f"({chromatic} chromatic, {achromatic} achromatic)"
            )
            if library is not None:
                print(
                    "Library:  "
                    f"{library.total_entries} entries, "
                    f"{len(library.filaments)} eligible, "
                    f"{library.skipped_unowned} unowned skipped, "
                    f"{library.skipped_filtered} type-filtered, "
                    f"{library.skipped_duplicate} optical duplicates skipped"
                )
        print(
            "Delta E76 (alpha-weighted): "
            f"mean {result.quality.mean_delta_e76:.2f}, "
            f"RMS {result.quality.rms_delta_e76:.2f}, "
            f"max {result.quality.max_delta_e76:.2f}"
        )
        print(f"RGBA SHA-256: {result.rgba_sha256}")
        print(f"Saved:    {output_path}")
        if arguments.hueforge_height_map:
            print(f"Layers:   {arguments.hueforge_height_map}")
        print(f"Time:     {elapsed:.2f}s")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
