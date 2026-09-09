# Calibration and repeatable print checks

Record RGB source (vendor SKU, measured swatch, or manual), TD source (for example
TD1S), spool/lot, date, printer, layer heights, substrate, lighting, and exposure
in **Advanced → Optical calibration → Calibration notes**. Notes travel with
profiles, projects, and reports. True Black is an artwork override and is reported
separately from original library prediction.

**TD sensitivity** varies one used spool at a time in both directions, holding
order, run lengths, output heights, and other spools fixed. The report shows
maximum CIELAB change over used output colors. It is a sensitivity diagnostic,
not an instrument accuracy claim or fitted physical correction. Zero disables
it; up to 32 used spools are evaluated.

Use this physical acceptance protocol:

1. Save the library, RGB/TD provenance, profile, PNG, layer map, and HFP. Turn
   True Black off for measured-library validation. Record the light preset and
   how it differs from the real viewing lamp.
2. Print single-filament stepped swatches at the intended first/regular layer
   heights, including repeat patches. Hold infill, extrusion, nozzle, substrate,
   viewing light, exposure, and white balance fixed.
3. Print contrasting pairs in both orders: especially tan/beige over red and
   red over tan/beige, plus white over dark green and the reverse. Include the
   same steps over neutral white and black controls. HueForge's feedback about
   substrate-dependent red blocking motivates this comparison; it does not
   establish a universal TD offset. Do not automatically lower every TD.
4. Compare measured RGB/Lab with predictions at each thickness. Keep raw
   measurements and repeat patches, channel-specific residuals, and Delta E76.
   Vendor RGB and measured TD are separate inputs with separate provenance.
5. Reserve a multicolor image and one pair from fitting. Evaluate held-out
   prints only after choosing any adjustment. Inspect skin/red/green boundaries
   and gradients under the same lighting.
6. Record results and limitations for the supported printer/material setup.
   Repeat after meaningful spool, geometry, or optical-model changes.

Native compatibility cannot establish spectral transmission, surface roughness,
or printed appearance. No new physical correction has been inferred from the
conversation with HueForge.
