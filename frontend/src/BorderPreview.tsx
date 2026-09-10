import type { BorderView } from "./types";
import "./border.css";

export function BorderPreview({
  border: b,
  imageURL,
}: {
  border: BorderView;
  imageURL?: string;
}) {
  const color = `rgb(${b.topRGB.join(",")})`;
  return (
    <figure className="border-preview" aria-label="HueForge border preview">
      {imageURL && (
        <svg
          viewBox={`0 0 ${b.outerWidthMm} ${b.outerHeightMm}`}
          role="img"
          aria-label="Rectangular frame around the complete image"
        >
          <rect width={b.outerWidthMm} height={b.outerHeightMm} fill={color} />
          <rect
            x={b.widthMm}
            y={b.widthMm}
            width={b.imageWidthMm}
            height={b.imageHeightMm}
            fill="var(--panel, #262b29)"
          />
          <image
            href={imageURL}
            x={b.widthMm}
            y={b.widthMm}
            width={b.imageWidthMm}
            height={b.imageHeightMm}
            preserveAspectRatio="none"
          />
        </svg>
      )}
      <figcaption>
        <strong>
          Frame: {b.outerWidthMm.toFixed(2)} × {b.outerHeightMm.toFixed(2)} mm
        </strong>
        <span>
          Image: {b.imageWidthMm.toFixed(2)} × {b.imageHeightMm.toFixed(2)} mm
        </span>
        <span>
          <i className="border-swatch" style={{ background: color }} />
          Border top: {b.heightMm.toFixed(2)} mm · layer {b.topLayer}
        </span>
        <span>Overall print height: {b.printHeightMm.toFixed(2)} mm</span>
        <span>
          Frame material: approximately {(b.volumeMm3 / 1000).toFixed(2)} cm³,
          in addition to the image
        </span>
        {b.extraLayers > 0 && (
          <span className="field-help">
            The final filament ({b.finalFilament}) continues for {b.extraLayers}{" "}
            border-only layers above the image. The image depth limit does not
            limit the border.
          </span>
        )}
        <span className="field-help">
          Nominal dimensions; HueForge rounds the mesh to its sampling grid. The
          frame stays rectangular around transparent images. Check that the
          image touches the frame where needed.
        </span>
      </figcaption>
    </figure>
  );
}
