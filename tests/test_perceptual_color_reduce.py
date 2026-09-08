import importlib.util
from io import StringIO
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

import numpy as np
from PIL import Image


SCRIPT_PATH = Path(__file__).parents[1] / "perceptual_color_reduce.py"
WORKSPACE_ROOT = SCRIPT_PATH.parent
SPEC = importlib.util.spec_from_file_location("perceptual_color_reduce", SCRIPT_PATH)
assert SPEC is not None and SPEC.loader is not None
REDUCER = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = REDUCER
SPEC.loader.exec_module(REDUCER)


def gradient_fixture(width=96, height=64):
    x = np.linspace(0, 255, width, dtype=np.uint8)[None, :]
    y = np.linspace(0, 255, height, dtype=np.uint8)[:, None]
    red = np.broadcast_to(x, (height, width))
    green = np.broadcast_to(y, (height, width))
    blue = ((red.astype(np.uint16) + green.astype(np.uint16)) // 2).astype(
        np.uint8
    )
    neutral = np.abs(red.astype(np.int16) - green.astype(np.int16)) < 20
    red = red.copy()
    green = green.copy()
    blue = blue.copy()
    red[neutral] = blue[neutral]
    green[neutral] = blue[neutral]
    alpha = np.broadcast_to(
        np.linspace(32, 255, width, dtype=np.uint8)[None, :],
        (height, width),
    ).copy()
    return np.dstack((red, green, blue, alpha))


def hueforge_record(
    name,
    color,
    td,
    uuid,
    *,
    owned=True,
    material_type="PLA",
    **extra,
):
    record = {
        "Brand": "Test Brand",
        "Color": color,
        "Name": name,
        "Owned": owned,
        "Tags": ["test"],
        "Transmissivity": td,
        "Type": material_type,
        "uuid": uuid,
    }
    record.update(extra)
    return record


class ColorConversionTests(unittest.TestCase):
    def test_srgb_lab_round_trip_is_within_one_byte(self):
        colors = np.asarray(
            (
                (0, 0, 0),
                (255, 255, 255),
                (255, 0, 0),
                (0, 255, 0),
                (0, 0, 255),
                (17, 93, 201),
                (128, 127, 126),
            ),
            dtype=np.uint8,
        )

        round_trip = REDUCER._lab_to_srgb(REDUCER._srgb_to_lab(colors))

        error = np.abs(round_trip.astype(np.int16) - colors.astype(np.int16))
        self.assertLessEqual(int(error.max()), 1)


class ReductionTests(unittest.TestCase):
    def options(self, **changes):
        values = {
            "population_colors": 4,
            "analysis_max_pixels": 1_000,
            "neutral_chroma": 8.0,
            "min_cluster_fraction": 0.0,
            "histogram_bits": 5,
            "max_iterations": 12,
            "chunk_pixels": 137,
            "preblur_sigma": 1.5,
        }
        values.update(changes)
        return REDUCER.ReductionOptions(**values)

    def test_analysis_size_matches_js_style_rounding(self):
        size = REDUCER.compute_analysis_size(4_000, 3_000, 6_291_456)

        scale = (6_291_456 / (4_000 * 3_000)) ** 0.5
        expected = (
            int(4_000 * scale + 0.5),
            int(3_000 * scale + 0.5),
        )
        self.assertEqual(size, expected)

    def test_analysis_size_never_exceeds_ceiling(self):
        for width, height, maximum in (
            (10_000, 1, 100),
            (16_969, 33_507, 6_291_456),
            (1, 10_000, 100),
        ):
            with self.subTest(width=width, height=height, maximum=maximum):
                analysis = REDUCER.compute_analysis_size(width, height, maximum)
                self.assertLessEqual(analysis[0] * analysis[1], maximum)
                self.assertGreaterEqual(analysis[0], 1)
                self.assertGreaterEqual(analysis[1], 1)

    def test_rejects_non_finite_numeric_options(self):
        for field in (
            "neutral_chroma",
            "min_cluster_fraction",
            "preblur_sigma",
        ):
            for value in (float("nan"), float("inf"), -float("inf")):
                with self.subTest(field=field, value=value):
                    options = self.options(**{field: value})
                    with self.assertRaises(REDUCER.ReductionError):
                        options.validate()

    def test_cluster_cull_repeats_after_survivor_refinement(self):
        rng = np.random.default_rng(4)
        for _ in range(5):
            count = int(rng.integers(5, 60))
            requested = int(rng.integers(2, min(12, count) + 1))
            fraction = float(rng.uniform(0.02, 0.35))
            points = rng.normal(size=(count, 3)) * rng.choice(
                [1, 5, 20], size=(count, 1)
            )
            weights = np.exp(rng.normal(size=count) * 2)

        _, masses = REDUCER._cluster_population(
            points, weights, requested, fraction, 50
        )
        threshold = weights.sum() * fraction
        self.assertTrue(len(masses) == 1 or np.all(masses >= threshold))

    def test_output_is_full_resolution_and_preserves_alpha(self):
        source_array = gradient_fixture()
        source = Image.fromarray(source_array, mode="RGBA")

        result = REDUCER.reduce_image(source, self.options())

        self.assertEqual(result.source_size, source.size)
        self.assertEqual(result.rgba.shape, source_array.shape)
        self.assertEqual(result.analysis_size, (38, 26))
        np.testing.assert_array_equal(result.rgba[..., 3], source_array[..., 3])

        visible_output = result.rgba[result.rgba[..., 3] > 0, :3]
        output_colors = {tuple(color) for color in np.unique(visible_output, axis=0)}
        palette_colors = {entry.rgb for entry in result.palette}
        self.assertTrue(output_colors.issubset(palette_colors))
        self.assertLessEqual(len(result.palette), 8)

    def test_result_is_deterministic_and_independent_of_chunk_size(self):
        source = Image.fromarray(gradient_fixture(), mode="RGBA")

        first = REDUCER.reduce_image(source, self.options(chunk_pixels=97))
        second = REDUCER.reduce_image(source, self.options(chunk_pixels=509))

        self.assertEqual(first.palette, second.palette)
        self.assertEqual(first.rgba_sha256, second.rgba_sha256)
        np.testing.assert_array_equal(first.rgba, second.rgba)

    def test_fully_transparent_pixels_keep_alpha_and_use_palette_rgb(self):
        source_array = np.zeros((5, 7, 4), dtype=np.uint8)
        source_array[..., :3] = (200, 50, 100)
        source = Image.fromarray(source_array, mode="RGBA")

        result = REDUCER.reduce_image(source, self.options())

        expected = np.broadcast_to(result.palette[0].rgb, result.rgba[..., :3].shape)
        np.testing.assert_array_equal(result.rgba[..., :3], expected)
        np.testing.assert_array_equal(result.rgba[..., 3], 0)

    def test_hidden_transparent_rgb_does_not_add_an_off_palette_color(self):
        source_array = gradient_fixture(30, 20)
        source_array[:, :5, 3] = 0
        source_array[:, :5, :3] = (1, 2, 3)
        result = REDUCER.reduce_image(
            Image.fromarray(source_array, mode="RGBA"), self.options()
        )

        output_colors = {
            tuple(color)
            for color in np.unique(result.rgba[..., :3].reshape(-1, 3), axis=0)
        }
        self.assertTrue(output_colors.issubset({entry.rgb for entry in result.palette}))

    def test_save_is_lossless_and_refuses_implicit_overwrite(self):
        source_array = gradient_fixture(24, 16)
        source = Image.fromarray(source_array, mode="RGBA")
        result = REDUCER.reduce_image(source, self.options(analysis_max_pixels=0))

        descriptor, temporary_name = tempfile.mkstemp(
            dir=WORKSPACE_ROOT, suffix="-perceptual-reduced.png"
        )
        os.close(descriptor)
        destination = Path(temporary_name)
        destination.unlink()
        try:
            REDUCER.save_result(
                result, destination, had_alpha=True, overwrite=False
            )
            with Image.open(destination) as saved:
                self.assertEqual(saved.size, source.size)
                self.assertEqual(saved.mode, "RGBA")
                np.testing.assert_array_equal(np.asarray(saved), result.rgba)

            with self.assertRaisesRegex(REDUCER.ReductionError, "already exists"):
                REDUCER.save_result(
                    result, destination, had_alpha=True, overwrite=False
                )
        finally:
            destination.unlink(missing_ok=True)

    def test_palette_report_is_valid_json_and_refuses_implicit_overwrite(self):
        result = REDUCER.reduce_image(
            Image.fromarray(gradient_fixture(20, 12), mode="RGBA"),
            self.options(analysis_max_pixels=0),
        )
        descriptor, temporary_name = tempfile.mkstemp(
            dir=WORKSPACE_ROOT, suffix="-perceptual-palette.json"
        )
        os.close(descriptor)
        destination = Path(temporary_name)
        destination.unlink()
        try:
            REDUCER.write_palette_json(result, destination)
            payload = json.loads(destination.read_text(encoding="utf-8"))
            self.assertEqual(len(payload["palette"]), len(result.palette))
            self.assertEqual(payload["rgba_sha256"], result.rgba_sha256)

            with self.assertRaisesRegex(REDUCER.ReductionError, "already exists"):
                REDUCER.write_palette_json(result, destination)
        finally:
            destination.unlink(missing_ok=True)

    def test_no_overwrite_commit_does_not_clobber_racing_file(self):
        result = REDUCER.reduce_image(
            Image.fromarray(gradient_fixture(12, 8), mode="RGBA"),
            self.options(analysis_max_pixels=0),
        )
        descriptor, temporary_name = tempfile.mkstemp(
            dir=WORKSPACE_ROOT, suffix="-perceptual-race.png"
        )
        os.close(descriptor)
        destination = Path(temporary_name)
        destination.unlink()
        real_link = os.link

        def create_racer_then_link(source, target):
            Path(target).write_bytes(b"racing writer won")
            return real_link(source, target)

        try:
            with mock.patch.object(
                REDUCER.os, "link", side_effect=create_racer_then_link
            ):
                with self.assertRaisesRegex(
                    REDUCER.ReductionError, "already exists"
                ):
                    REDUCER.save_result(
                        result, destination, had_alpha=True, overwrite=False
                    )
            self.assertEqual(destination.read_bytes(), b"racing writer won")
        finally:
            destination.unlink(missing_ok=True)


class HueForgeTests(unittest.TestCase):
    def reduction_options(self, **changes):
        values = {
            "population_colors": 2,
            "analysis_max_pixels": 0,
            "neutral_chroma": 8.0,
            "min_cluster_fraction": 0.0,
            "histogram_bits": 4,
            "max_iterations": 8,
            "chunk_pixels": 17,
            "preblur_sigma": 0.0,
        }
        values.update(changes)
        return REDUCER.ReductionOptions(**values)

    def hueforge_options(self, **changes):
        values = {
            "layer_height_mm": 0.1,
            "base_depth_mm": 0.2,
            "max_depth_mm": 0.6,
            "analysis_colors": 4,
            "beam_width": 8,
            "max_perceived_colors": 8,
            "td_transmission": 0.05,
            "td_scale": 1.0,
            "base_transmission_limit": 1.0,
        }
        values.update(changes)
        return REDUCER.HueForgeOptions(**values)

    @staticmethod
    def write_library(records):
        descriptor, temporary_name = tempfile.mkstemp(
            dir=WORKSPACE_ROOT, suffix="-hueforge-library.json"
        )
        os.close(descriptor)
        path = Path(temporary_name)
        path.write_text(
            json.dumps({"Filaments": records}), encoding="utf-8"
        )
        return path

    def grayscale_fixture(self):
        levels = np.asarray(
            (0, 32, 64, 96, 128, 160, 192, 224, 255), dtype=np.uint8
        )
        rgb = np.broadcast_to(levels[None, :, None], (5, levels.size, 3)).copy()
        alpha = np.broadcast_to(
            np.asarray((32, 64, 96, 128, 160, 192, 224, 240, 255), dtype=np.uint8)[
                None, :, None
            ],
            (5, levels.size, 1),
        ).copy()
        return np.concatenate((rgb, alpha), axis=2)

    def two_filament_reduction(self):
        records = [
            hueforge_record("Black", "#000000", 0.5, "black-filament"),
            hueforge_record("White", "#FFFFFF", 1.0, "white-filament"),
        ]
        path = self.write_library(records)
        try:
            library = REDUCER.load_hueforge_library(path)
        finally:
            path.unlink(missing_ok=True)
        source_array = self.grayscale_fixture()
        result = REDUCER.reduce_image_hueforge(
            Image.fromarray(source_array, mode="RGBA"),
            self.reduction_options(),
            self.hueforge_options(),
            library,
        )
        return source_array, library, result

    def test_hueforge_library_filters_invalid_secondary_and_duplicates(self):
        records = [
            hueforge_record("Black", "#000000", 0.5, "black"),
            hueforge_record("White", "#FFFFFF", 1.0, "white"),
            hueforge_record("Blue", "#0000FF", 1.2, "blue", owned=False),
            hueforge_record(
                "Gray PETG", "#777777", 0.7, "gray", material_type="PETG"
            ),
            hueforge_record(
                "Silk",
                "#AA00AA",
                1.5,
                "silk",
                Secondary_Color="#00AAAA",
                Secondary_Strength=0.5,
                Secondary_Type="coextrusion",
            ),
            hueforge_record("Broken", "not-a-color", 1.0, "broken"),
            hueforge_record("Black duplicate", "#000000", 0.5, "black-copy"),
        ]
        path = self.write_library(records)
        try:
            default_library = REDUCER.load_hueforge_library(path)
            expanded_library = REDUCER.load_hueforge_library(
                path,
                include_unowned=True,
                material_types=("pla",),
                allow_secondary=True,
            )
        finally:
            path.unlink(missing_ok=True)

        self.assertEqual(
            [filament.name for filament in default_library.filaments],
            ["Black", "White", "Gray PETG"],
        )
        self.assertEqual(default_library.skipped_unowned, 1)
        self.assertEqual(default_library.skipped_secondary, 1)
        self.assertEqual(default_library.skipped_invalid, 1)
        self.assertEqual(default_library.skipped_duplicate, 1)
        self.assertEqual(
            [filament.name for filament in expanded_library.filaments],
            ["Black", "White", "Blue", "Silk"],
        )
        self.assertTrue(expanded_library.filaments[-1].has_secondary_color)
        self.assertEqual(expanded_library.skipped_filtered, 1)
        self.assertEqual(expanded_library.skipped_duplicate, 1)

    def test_hueforge_library_rejects_an_invalid_document(self):
        descriptor, temporary_name = tempfile.mkstemp(
            dir=WORKSPACE_ROOT, suffix="-invalid-hueforge-library.json"
        )
        os.close(descriptor)
        path = Path(temporary_name)
        try:
            path.write_text(json.dumps({"NotFilaments": []}), encoding="utf-8")
            with self.assertRaisesRegex(REDUCER.ReductionError, "Filaments"):
                REDUCER.load_hueforge_library(path)
        finally:
            path.unlink(missing_ok=True)

    def test_hueforge_library_rejects_missing_or_unknown_owned_values(self):
        missing_owned = hueforge_record(
            "Missing Owned", "#224466", 0.8, "missing-owned"
        )
        missing_owned.pop("Owned")
        records = [
            hueforge_record("Valid", "#000000", 0.5, "valid"),
            hueforge_record(
                "Explicitly unowned",
                "#FFFFFF",
                1.0,
                "unowned",
                owned=False,
            ),
            missing_owned,
            hueforge_record(
                "Unknown Owned", "#AA0000", 0.9, "unknown-owned", owned="maybe"
            ),
        ]
        path = self.write_library(records)
        try:
            library = REDUCER.load_hueforge_library(
                path, include_unowned=True
            )
        finally:
            path.unlink(missing_ok=True)

        self.assertEqual(
            [filament.name for filament in library.filaments],
            ["Valid", "Explicitly unowned"],
        )
        self.assertEqual(library.skipped_invalid, 2)
        self.assertEqual(library.skipped_unowned, 0)

    def test_hueforge_options_reject_rounded_invalid_layer_counts(self):
        invalid_options = (
            (
                self.hueforge_options(
                    layer_height_mm=1.0,
                    base_depth_mm=1e-9,
                    max_depth_mm=1.0,
                ),
                "at least one layer",
            ),
            (
                self.hueforge_options(
                    layer_height_mm=1.0,
                    base_depth_mm=1.0,
                    max_depth_mm=1.0 + 5e-9,
                ),
                "leave at least one layer",
            ),
        )
        for options, message in invalid_options:
            with self.subTest(message=message):
                with self.assertRaisesRegex(REDUCER.ReductionError, message):
                    options.validate()

    def test_hueforge_default_front_lit_td_scale_is_one_tenth(self):
        self.assertEqual(REDUCER.HueForgeOptions().td_scale, 0.1)

    def test_hueforge_layer_simulation_uses_exact_td_transmission(self):
        filament = REDUCER.HueForgeFilament(
            brand="Test Brand",
            name="Red",
            rgb=(255, 0, 0),
            td_mm=1.0,
            material_type="PLA",
            uuid="red",
            owned=True,
            tags=(),
            source_index=0,
        )
        options = self.hueforge_options(
            layer_height_mm=0.5,
            base_depth_mm=0.5,
            max_depth_mm=1.0,
            td_transmission=0.25,
            td_scale=1.0,
        )

        first = REDUCER._simulate_hueforge_layer(
            np.asarray((0.0, 0.0, 0.0)), filament, options
        )
        second = REDUCER._simulate_hueforge_layer(first, filament, options)

        np.testing.assert_allclose(first, (0.5, 0.0, 0.0), atol=1e-12)
        np.testing.assert_allclose(second, (0.75, 0.0, 0.0), atol=1e-12)

    def test_reachable_palette_finds_the_exact_two_color_subset(self):
        candidate_rgb = np.asarray(
            ((0, 0, 0), (225, 225, 225), (248, 248, 248)), dtype=np.uint8
        )
        target_rgb = candidate_rgb[[0, 2]]

        selected, masses, rms = REDUCER._select_reachable_palette(
            REDUCER._srgb_to_lab(candidate_rgb).astype(np.float64),
            REDUCER._srgb_to_lab(target_rgb).astype(np.float64),
            np.asarray((0.5, 0.5), dtype=np.float64),
            maximum_colors=2,
            minimum_fraction=0.0,
        )

        self.assertEqual(selected, [0, 2])
        np.testing.assert_allclose(masses, (0.5, 0.5), atol=1e-12)
        self.assertEqual(rms, 0.0)

    def test_terminal_stack_is_reranked_for_the_emitted_palette_cap(self):
        filaments = tuple(
            REDUCER.HueForgeFilament(
                brand="Test Brand",
                name=name,
                rgb=rgb,
                td_mm=1.0,
                material_type="PLA",
                uuid=name,
                owned=True,
                tags=(),
                source_index=index,
            )
            for index, (name, rgb) in enumerate(
                (
                    ("Black", (0, 0, 0)),
                    ("White", (255, 255, 255)),
                    ("Gray 119", (119, 119, 119)),
                )
            )
        )
        library = REDUCER.HueForgeLibrary(
            filaments=filaments,
            total_entries=3,
            skipped_unowned=0,
            skipped_filtered=0,
            skipped_invalid=0,
            skipped_secondary=0,
            skipped_duplicate=0,
            sha256="test",
        )
        targets = (
            REDUCER.PaletteEntry((0, 0, 0), "achromatic", 0.5),
            REDUCER.PaletteEntry((255, 255, 255), "achromatic", 0.5),
        )

        palette, plan = REDUCER._optimize_hueforge_stack(
            targets,
            library,
            requested_filaments=2,
            reduction_options=self.reduction_options(
                population_colors=2, min_cluster_fraction=0.0
            ),
            hueforge_options=self.hueforge_options(
                analysis_colors=2, max_perceived_colors=1
            ),
        )

        self.assertEqual([entry.rgb for entry in palette], [(119, 119, 119)])
        self.assertEqual([run.filament.name for run in plan.runs], ["Gray 119"])
        self.assertAlmostEqual(plan.weighted_rms_delta_e76, 50.0, places=3)

    def test_larger_filament_budget_cannot_force_a_worse_stack(self):
        filaments = tuple(
            REDUCER.HueForgeFilament(
                brand="Test Brand",
                name=name,
                rgb=rgb,
                td_mm=1.0,
                material_type="PLA",
                uuid=name,
                owned=True,
                tags=(),
                source_index=index,
            )
            for index, (name, rgb) in enumerate(
                (
                    ("Black", (0, 0, 0)),
                    ("White", (255, 255, 255)),
                    ("Red", (255, 0, 0)),
                )
            )
        )
        library = REDUCER.HueForgeLibrary(
            filaments=filaments,
            total_entries=3,
            skipped_unowned=0,
            skipped_filtered=0,
            skipped_invalid=0,
            skipped_secondary=0,
            skipped_duplicate=0,
            sha256="test",
        )
        targets = (
            REDUCER.PaletteEntry((0, 0, 0), "achromatic", 0.499),
            REDUCER.PaletteEntry((248, 248, 248), "achromatic", 0.499),
            REDUCER.PaletteEntry((225, 225, 225), "achromatic", 0.002),
        )
        hueforge_options = self.hueforge_options(
            layer_height_mm=1.0,
            base_depth_mm=1.0,
            max_depth_mm=3.0,
            analysis_colors=3,
            td_transmission=0.25,
            td_scale=1.0,
            base_transmission_limit=1.0,
        )

        palette, plan = REDUCER._optimize_hueforge_stack(
            targets,
            library,
            requested_filaments=3,
            reduction_options=self.reduction_options(
                population_colors=3, min_cluster_fraction=0.0
            ),
            hueforge_options=hueforge_options,
        )

        self.assertEqual([run.filament.name for run in plan.runs], ["Black", "White"])
        self.assertEqual([run.layers for run in plan.runs], [1, 2])
        self.assertEqual({entry.rgb for entry in palette}, {target.rgb for target in targets})
        self.assertAlmostEqual(plan.weighted_rms_delta_e76, 0.0, places=6)

    def test_solid_black_hueforge_plan_trims_unused_filaments_and_layers(self):
        records = [
            hueforge_record("Black", "#000000", 0.5, "black"),
            hueforge_record("White", "#FFFFFF", 1.0, "white"),
            hueforge_record("Red", "#FF0000", 0.8, "red"),
        ]
        path = self.write_library(records)
        try:
            library = REDUCER.load_hueforge_library(path)
        finally:
            path.unlink(missing_ok=True)
        source = Image.new("RGBA", (4, 3), (0, 0, 0, 255))
        hueforge_options = self.hueforge_options()

        result = REDUCER.reduce_image_hueforge(
            source,
            self.reduction_options(population_colors=3),
            hueforge_options,
            library,
        )

        plan = result.hueforge_plan
        self.assertIsNotNone(plan)
        self.assertEqual(len(plan.runs), 1)
        self.assertEqual(plan.runs[0].filament.rgb, (0, 0, 0))
        self.assertEqual(plan.runs[0].layers, hueforge_options.base_layers)
        self.assertEqual(plan.planned_depth_mm, hueforge_options.base_depth_mm)
        self.assertEqual(len(plan.layer_colors), hueforge_options.base_layers)
        self.assertEqual({layer.rgb for layer in plan.layer_colors}, {(0, 0, 0)})
        self.assertEqual([entry.rgb for entry in result.palette], [(0, 0, 0)])

    def test_hueforge_reduction_is_deterministic_and_globally_reachable(self):
        source_array, library, first = self.two_filament_reduction()
        second = REDUCER.reduce_image_hueforge(
            Image.fromarray(source_array, mode="RGBA"),
            self.reduction_options(chunk_pixels=31),
            self.hueforge_options(),
            library,
        )

        self.assertEqual(first.palette, second.palette)
        self.assertEqual(first.hueforge_plan, second.hueforge_plan)
        self.assertEqual(first.rgba_sha256, second.rgba_sha256)
        np.testing.assert_array_equal(first.rgba, second.rgba)
        self.assertEqual(
            first.source_size, (source_array.shape[1], source_array.shape[0])
        )
        self.assertEqual(first.rgba.shape, source_array.shape)
        np.testing.assert_array_equal(first.rgba[..., 3], source_array[..., 3])

        plan = first.hueforge_plan
        self.assertIsNotNone(plan)
        selected = {run.filament.uuid for run in plan.runs}
        self.assertLessEqual(
            len(selected), self.reduction_options().population_colors
        )
        self.assertGreater(len({entry.rgb for entry in first.palette}), len(selected))

        reachable = {(layer.layer, layer.rgb) for layer in plan.layer_colors}
        self.assertTrue(
            all(
                (entry.stack_layer, entry.rgb) in reachable
                for entry in first.palette
            )
        )
        for previous, current in zip(plan.runs, plan.runs[1:]):
            self.assertEqual(previous.end_layer + 1, current.start_layer)

    def test_guided_reduction_uses_owned_anchors_and_pairwise_hues(self):
        records = [
            hueforge_record("Black", "#000000", 0.5, "black"),
            hueforge_record("White", "#FFFFFF", 1.0, "white"),
            hueforge_record(
                "Unowned Gray", "#808080", 0.7, "gray", owned=False
            ),
        ]
        path = self.write_library(records)
        try:
            library = REDUCER.load_hueforge_library(path)
        finally:
            path.unlink(missing_ok=True)
        source_array = self.grayscale_fixture()
        first = REDUCER.reduce_image_hueforge_guided(
            Image.fromarray(source_array, mode="RGBA"),
            self.reduction_options(population_colors=2),
            self.hueforge_options(),
            library,
        )
        second = REDUCER.reduce_image_hueforge_guided(
            Image.fromarray(source_array, mode="RGBA"),
            self.reduction_options(population_colors=2, chunk_pixels=31),
            self.hueforge_options(),
            library,
        )

        self.assertIsNone(first.hueforge_plan)
        self.assertIsNone(first.stack_layer_map)
        self.assertIsNotNone(first.hueforge_guidance)
        guidance = first.hueforge_guidance
        self.assertLessEqual(len(guidance.selected_filaments), 2)
        self.assertTrue(all(filament.owned for filament in guidance.selected_filaments))
        self.assertNotIn("Unowned Gray", {f.name for f in guidance.selected_filaments})
        self.assertGreater(len(first.palette), len(guidance.selected_filaments))
        self.assertIn(
            "pairwise-perceived-hue",
            {color.reference_kind for color in guidance.colors},
        )
        self.assertEqual(guidance.guidance_strength, 0.8)
        self.assertTrue(
            any(color.rgb != color.reference_rgb for color in guidance.colors)
        )
        self.assertEqual(
            {entry.rgb for entry in first.palette},
            {color.rgb for color in guidance.colors},
        )
        self.assertTrue(
            all(
                entry.stack_layer is None
                and entry.stack_height_mm is None
                and entry.top_filament_position is None
                for entry in first.palette
            )
        )
        np.testing.assert_array_equal(first.rgba[..., 3], source_array[..., 3])
        self.assertEqual(first.palette, second.palette)
        self.assertEqual(first.hueforge_guidance, second.hueforge_guidance)
        self.assertEqual(first.rgba_sha256, second.rgba_sha256)

    def test_guided_anchor_selection_favors_the_relevant_owned_color(self):
        records = [
            hueforge_record("Red", "#FF0000", 1.0, "red"),
            hueforge_record("Blue", "#0000FF", 1.0, "blue"),
            hueforge_record("Green", "#00FF00", 1.0, "green"),
        ]
        path = self.write_library(records)
        try:
            library = REDUCER.load_hueforge_library(path)
        finally:
            path.unlink(missing_ok=True)
        source_rgb = (240, 35, 25)
        source = Image.new("RGBA", (5, 4), (*source_rgb, 255))
        guided = REDUCER.reduce_image_hueforge_guided(
            source,
            self.reduction_options(population_colors=1),
            self.hueforge_options(),
            library,
        )
        snapped = REDUCER.reduce_image_hueforge_guided(
            source,
            self.reduction_options(population_colors=1),
            self.hueforge_options(),
            library,
            guidance_strength=1.0,
        )

        self.assertEqual(
            [
                filament.name
                for filament in guided.hueforge_guidance.selected_filaments
            ],
            ["Red"],
        )
        reference_rgb = guided.hueforge_guidance.colors[0].reference_rgb
        comparison_lab = REDUCER._srgb_to_lab(
            np.asarray((source_rgb, guided.palette[0].rgb, reference_rgb))
        )
        source_distance = float(np.linalg.norm(comparison_lab[0] - comparison_lab[2]))
        guided_distance = float(np.linalg.norm(comparison_lab[1] - comparison_lab[2]))
        self.assertLess(guided_distance, source_distance)
        self.assertNotEqual(guided.palette[0].rgb, reference_rgb)
        self.assertEqual([entry.rgb for entry in snapped.palette], [(255, 0, 0)])

    def test_guided_plan_prunes_anchors_unused_by_the_emitted_palette(self):
        filament_values = (
            ((85, 27, 39), 2.0),
            ((83, 38, 110), 0.5),
            ((13, 177, 56), 0.5),
            ((156, 232, 20), 2.0),
            ((141, 20, 90), 1.0),
        )
        records = [
            hueforge_record(
                f"F{index}",
                "#{:02X}{:02X}{:02X}".format(*rgb),
                td,
                f"f-{index}",
            )
            for index, (rgb, td) in enumerate(filament_values)
        ]
        path = self.write_library(records)
        try:
            library = REDUCER.load_hueforge_library(path)
        finally:
            path.unlink(missing_ok=True)
        targets = tuple(
            REDUCER.PaletteEntry(
                rgb=rgb,
                population="chromatic",
                analysis_fraction=fraction,
            )
            for rgb, fraction in (
                ((178, 253, 174), 0.06121662246005443),
                ((239, 243, 23), 0.3781099623757967),
                ((241, 87, 225), 0.06120905224147089),
                ((224, 151, 140), 0.3302562988099689),
                ((63, 95, 213), 0.016262650825708387),
                ((223, 61, 52), 0.15294541328700062),
            )
        )
        _, guidance = REDUCER._optimize_hueforge_guidance(
            targets,
            library,
            requested_filaments=4,
            reduction_options=self.reduction_options(population_colors=4),
            hueforge_options=self.hueforge_options(
                analysis_colors=6,
                max_perceived_colors=2,
            ),
        )

        self.assertEqual(
            [filament.name for filament in guidance.selected_filaments],
            ["F3", "F4"],
        )
        referenced_positions = {
            position
            for color in guidance.colors
            for position in color.filament_positions
        }
        self.assertEqual(referenced_positions, {1, 2})

    def test_guided_palette_report_is_not_a_stack_plan(self):
        records = [
            hueforge_record("Black", "#000000", 0.5, "black"),
            hueforge_record("White", "#FFFFFF", 1.0, "white"),
        ]
        path = self.write_library(records)
        try:
            library = REDUCER.load_hueforge_library(path)
        finally:
            path.unlink(missing_ok=True)
        result = REDUCER.reduce_image_hueforge_guided(
            Image.fromarray(self.grayscale_fixture(), mode="RGBA"),
            self.reduction_options(population_colors=2),
            self.hueforge_options(),
            library,
        )
        descriptor, temporary_name = tempfile.mkstemp(
            dir=WORKSPACE_ROOT, suffix="-hueforge-guided.json"
        )
        os.close(descriptor)
        destination = Path(temporary_name)
        destination.unlink()
        try:
            REDUCER.write_palette_json(result, destination)
            payload = json.loads(destination.read_text(encoding="utf-8"))
        finally:
            destination.unlink(missing_ok=True)

        self.assertEqual(payload["mode"], "hueforge-library-guided")
        self.assertIn("hueforge_guidance", payload)
        self.assertNotIn("hueforge_stack", payload)
        self.assertEqual(
            payload["hueforge_guidance"]["library_sha256"], library.sha256
        )
        self.assertTrue(payload["hueforge_guidance"]["selected_filaments"])
        self.assertTrue(payload["hueforge_guidance"]["guided_palette_sources"])
        self.assertEqual(payload["hueforge_guidance"]["guidance_strength"], 0.8)
        self.assertFalse(
            payload["hueforge_guidance"]["global_stack_guaranteed"]
        )
        self.assertIn(
            "reference_rgb",
            payload["hueforge_guidance"]["guided_palette_sources"][0],
        )
        self.assertIn(
            "reference_kind",
            payload["hueforge_guidance"]["guided_palette_sources"][0],
        )

    def test_hueforge_palette_report_contains_the_printable_plan(self):
        _, library, result = self.two_filament_reduction()
        descriptor, temporary_name = tempfile.mkstemp(
            dir=WORKSPACE_ROOT, suffix="-hueforge-palette.json"
        )
        os.close(descriptor)
        destination = Path(temporary_name)
        destination.unlink()
        try:
            REDUCER.write_palette_json(result, destination)
            payload = json.loads(destination.read_text(encoding="utf-8"))
        finally:
            destination.unlink(missing_ok=True)

        self.assertEqual(payload["mode"], "hueforge-stack")
        plan = payload["hueforge_stack"]
        self.assertEqual(plan["library_sha256"], library.sha256)
        self.assertEqual(plan["requested_filaments"], 2)
        self.assertEqual(plan["model"], REDUCER.HUEFORGE_STACK_MODEL)
        self.assertEqual(
            len(plan["runs_bottom_to_top"]), len(result.hueforge_plan.runs)
        )
        self.assertEqual(
            len(plan["layer_colors"]), len(result.hueforge_plan.layer_colors)
        )
        self.assertTrue(
            all("stack_layer" in entry for entry in payload["palette"])
        )
        self.assertEqual(plan["layer_index_base"], 1)
        self.assertIn("layer_index_map", plan)

    def test_hueforge_height_map_round_trips_exact_layer_indices(self):
        _, _, result = self.two_filament_reduction()
        self.assertIsNotNone(result.stack_layer_map)
        palette_layers = {entry.stack_layer for entry in result.palette}
        self.assertTrue(set(np.unique(result.stack_layer_map)) <= palette_layers)

        descriptor, temporary_name = tempfile.mkstemp(
            dir=WORKSPACE_ROOT, suffix="-hueforge-layers.png"
        )
        os.close(descriptor)
        destination = Path(temporary_name)
        destination.unlink()
        try:
            REDUCER.save_stack_layer_map(result, destination)
            with Image.open(destination) as opened:
                saved = np.asarray(opened)
            np.testing.assert_array_equal(saved, result.stack_layer_map)
            with self.assertRaisesRegex(REDUCER.ReductionError, "already exists"):
                REDUCER.save_stack_layer_map(result, destination)
        finally:
            destination.unlink(missing_ok=True)

    def test_hueforge_cli_writes_preview_report_and_layer_map(self):
        records = [
            hueforge_record("Black", "#000000", 0.5, "black"),
            hueforge_record("White", "#FFFFFF", 1.0, "white"),
            hueforge_record(
                "Unowned Gray", "#808080", 0.7, "gray", owned=False
            ),
        ]
        library_path = self.write_library(records)
        paths = []
        for suffix in ("-input.png", "-output.png", "-layers.png", "-plan.json"):
            descriptor, temporary_name = tempfile.mkstemp(
                dir=WORKSPACE_ROOT, suffix=suffix
            )
            os.close(descriptor)
            path = Path(temporary_name)
            path.unlink()
            paths.append(path)
        source_path, output_path, layer_path, report_path = paths
        Image.fromarray(self.grayscale_fixture(), mode="RGBA").save(source_path)

        try:
            return_code = REDUCER.main(
                (
                    str(source_path),
                    "-o",
                    str(output_path),
                    "--hueforge-library",
                    str(library_path),
                    "--hueforge-stack",
                    "--colors",
                    "2",
                    "--hueforge-layer-height",
                    "0.1",
                    "--hueforge-base-depth",
                    "0.2",
                    "--hueforge-max-depth",
                    "0.6",
                    "--hueforge-analysis-colors",
                    "4",
                    "--hueforge-beam-width",
                    "8",
                    "--hueforge-td-scale",
                    "1",
                    "--hueforge-base-transmission-limit",
                    "1",
                    "--hueforge-height-map",
                    str(layer_path),
                    "--palette-json",
                    str(report_path),
                    "--quiet",
                )
            )
            self.assertEqual(return_code, 0)
            self.assertTrue(output_path.is_file())
            self.assertTrue(layer_path.is_file())
            self.assertTrue(report_path.is_file())
            report = json.loads(report_path.read_text(encoding="utf-8"))
            self.assertEqual(report["mode"], "hueforge-stack")
            self.assertEqual(
                report["hueforge_stack"]["layer_index_base"], 1
            )
        finally:
            library_path.unlink(missing_ok=True)
            for path in paths:
                path.unlink(missing_ok=True)

    def test_hueforge_cli_library_defaults_to_guided_preprocessing(self):
        records = [
            hueforge_record("Black", "#000000", 0.5, "black"),
            hueforge_record("White", "#FFFFFF", 1.0, "white"),
        ]
        library_path = self.write_library(records)
        paths = []
        for suffix in ("-guided-input.png", "-guided-output.png", "-guided.json"):
            descriptor, temporary_name = tempfile.mkstemp(
                dir=WORKSPACE_ROOT, suffix=suffix
            )
            os.close(descriptor)
            path = Path(temporary_name)
            path.unlink()
            paths.append(path)
        source_path, output_path, report_path = paths
        Image.fromarray(self.grayscale_fixture(), mode="RGBA").save(source_path)
        try:
            return_code = REDUCER.main(
                (
                    str(source_path),
                    "-o",
                    str(output_path),
                    "--hueforge-library",
                    str(library_path),
                    "--colors",
                    "2",
                    "--palette-json",
                    str(report_path),
                    "--quiet",
                )
            )
            self.assertEqual(return_code, 0)
            payload = json.loads(report_path.read_text(encoding="utf-8"))
            self.assertEqual(payload["mode"], "hueforge-library-guided")
            self.assertIn("hueforge_guidance", payload)
            self.assertNotIn("hueforge_stack", payload)
            selected = payload["hueforge_guidance"]["selected_filaments"]
            self.assertLessEqual(len(selected), 2)
            self.assertTrue(all(filament["owned"] for filament in selected))
        finally:
            library_path.unlink(missing_ok=True)
            for path in paths:
                path.unlink(missing_ok=True)

    def test_stack_only_cli_options_require_explicit_stack_mode(self):
        with mock.patch.object(sys, "stderr", new=StringIO()) as error_output:
            return_code = REDUCER.main(
                (
                    "missing.png",
                    "--hueforge-library",
                    "library.json",
                    "--hueforge-height-map",
                    "layers.png",
                )
            )
        self.assertEqual(return_code, 1)
        self.assertIn("requires --hueforge-stack", error_output.getvalue())

        with mock.patch.object(sys, "stderr", new=StringIO()) as error_output:
            return_code = REDUCER.main(("missing.png", "--hueforge-stack"))
        self.assertEqual(return_code, 1)
        self.assertIn("require --hueforge-library", error_output.getvalue())


if __name__ == "__main__":
    unittest.main()
