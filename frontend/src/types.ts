export type HueForgeOptions = {
  searchEffort?: "" | "preview" | "refine";
  requiredFilaments?: string;
  baseFilament?: string;
  highlightFilament?: string;
  surfaceColorTolerance?: number;
  depthTolerance?: number;
  tdSensitivityPercent?: number;
  opticalModel: "hueforge-0.9.4.3-frontlit-v1" | "legacy-exponential";
  firstLayerHeight: number;
  lightPreset: "hueforge-default" | "neutral-white" | "warm-white" | "";
  layerHeight: number;
  baseDepth: number;
  maxDepth: number;
  autoDepth: boolean;
  reduceShowThrough: boolean;
  analysisColors: number;
  beamWidth: number;
  maxRuns: number;
  meshMode: "" | "color-match" | "combo" | "color-aware" | "color-pop";
  meshCore:
    | ""
    | "planned-colors"
    | "filament-blends"
    | "compact-blends"
    | "legacy-flat";
  exportWidthMm: number;
  meshDetailMm: number;
  maxPerceivedColors: number;
  tdTransmission: number;
  tdScale: number;
  baseTransmissionLimit: number;
};
export type ColorPopOptions = {
  enabled: boolean;
  selection: "existing" | "selected";
  colors: string;
  hueTolerance: number;
  grayTolerance: number;
  colorPercent: number;
  grayOnTop: boolean;
  gapLayers: number;
};
export const defaultColorPop: ColorPopOptions = {
  enabled: false,
  selection: "existing",
  colors: "",
  hueTolerance: 25,
  grayTolerance: 8,
  colorPercent: 50,
  grayOnTop: false,
  gapLayers: 1,
};
export type ColorPopInfo = {
  colorFraction: number;
  grayFraction: number;
  selectionPng: string;
  warning?: string;
  colorLayers: [number, number];
  grayLayers: [number, number];
  gapLayers: number;
  qualityReference: string;
};
export type Options = {
  colorPop: ColorPopOptions;
  protectedColors?: string;
  calibrationNote?: string;
  colors: number;
  totalColors: boolean;
  colorPriority: "" | "balanced" | "distinctive" | "vivid";
  analysisMaxPixels: number;
  neutralChroma: number;
  minClusterFraction: number;
  histogramBits: number;
  iterations: number;
  preblurSigma: number;
  smoothingColorSigma: number;
  legacyColorPipeline: boolean;
  mode: "standard" | "guided" | "stack";
  guidanceStrength: number;
  trueBlack: boolean;
  preserveDetails: boolean;
  hueforge: HueForgeOptions;
};
export type Filter = {
  includeUnowned: boolean;
  materialTypes: string[];
  allowSecondary: boolean;
  avoidSilkMetallic: boolean;
  excludedIds: number[];
};
export type Filament = {
  key?: string;
  brand: string;
  name: string;
  rgb: number[];
  hex: string;
  td: number;
  material: string;
  uuid: string;
  owned: boolean;
  sourceIndex: number;
  secondary: boolean;
  libraryRGB?: number[];
};
export type Library = {
  filaments: Filament[];
  total: number;
  skippedUnowned: number;
  skippedInvalid: number;
  skippedFiltered: number;
  skippedFinish: number;
  skippedSecondary: number;
  skippedDuplicate: number;
  sha256: string;
};
export type Source = {
  name: string;
  path: string;
  width: number;
  height: number;
  uniqueColors: number;
  url: string;
  revision: number;
  demo: boolean;
  metadata: {
    format: string;
    dpi: number[];
    colorProfile: string;
    warnings: string[];
  };
};
export type Preset = { name: string; options: Options };
export type SettingsProfile = {
  format: "ColorNinja settings";
  schemaVersion: number;
  name: string;
  options: Options;
  filter: Filter;
  librarySHA256?: string;
};
export type Preferences = {
  advanced: boolean;
  checkOnStartup: boolean;
  includePrereleases: boolean;
  exportProfile: boolean;
};
export const defaultPreferences: Preferences = {
  advanced: false,
  checkOnStartup: true,
  includePrereleases: false,
  exportProfile: false,
};
export type UpdateResult = {
  currentVersion: string;
  latestVersion: string;
  available: boolean;
  prerelease: boolean;
  includePrereleases: boolean;
  releaseURL: string;
  checkedAt: string;
};
export type Snapshot = {
  preview?: Preview;
  source: Source;
  settings: {
    comparisons?: Comparison[];
    preferences: Preferences;
    options: Options;
    libraryPath: string;
    filter: Filter;
    presets: Preset[];
    recent: string[];
  };
  library: Library | null;
  warning: string;
};
export type PaletteEntry = {
  rgb: number[];
  hex: string;
  population: string;
  analysisFraction: number;
  pixelFraction: number;
  stackLayer?: number;
  stackHeight?: number;
  topPosition?: number;
};
export type StackRun = {
  position: number;
  filament: Filament;
  layers: number;
  startLayer: number;
  endLayer: number;
  startHeight: number;
  endHeight: number;
};
export type Result = {
  colorPop?: ColorPopInfo;
  calibration?: {
    note?: string;
    trueBlackOverride: boolean;
    variationPercent: number;
    truncated?: boolean;
    sensitivity?: { key: string; name: string; maxDeltaE76: number }[];
  };
  surfaceView?: SurfaceView;
  palette: PaletteEntry[];
  sourceSize: number[];
  analysisSize: number[];
  uniqueColors: number;
  quality: { meanDeltaE76: number; rmsDeltaE76: number; maxDeltaE76: number };
  rgbaSHA256: string;
  guidance?: {
    colors: {
      rgb: number[];
      sourceRGB: number[];
      referenceRGB: number[];
      referenceKind: string;
      filamentPositions: number[];
      analysisFraction: number;
    }[];
    selectedFilaments: Filament[];
    strength: number;
    candidateColorCount: number;
    globalStackGuaranteed: false;
  };
  stack?: {
    uniqueFilaments: number;
    runs: StackRun[];
    plannedDepth: number;
    weightedRmsDeltaE76: number;
    layerColors: { rgb: number[]; layer: number }[];
    surface?: {
      boundaryPairs: number;
      meanHeightJumpMm: number;
      rmsColorDetour: number;
      penalty: number;
    };
    depthSelection?: {
      hardMaximum: number;
      printableMaximum: number;
      comparedDepths: number;
      bestScore: number;
      selectedScore: number;
      tolerancePercent: number;
      scoreMetric: string;
    };
  };
  stackView?: {
    meshMode: string;
    meshCore: string;
    optimization?: {
      strategy: string;
      entries: number;
      originalEntries: number;
      disabledLayers: number;
      originalDisabledLayers: number;
      minTD: number;
      maxTD: number;
    };
    hasMeshCore: boolean;
    layers: {
      layer: number;
      height: number;
      runPosition: number;
      predictedRGB: number[];
      meshRGB?: number[];
      meshEnabled: boolean;
      pixelFraction: number;
    }[];
  };
};
export type Preview = {
  reusedStages?: string[];
  id: number;
  revision: number;
  url: string;
  result: Result;
  options: Options;
  seconds: number;
  warning?: string;
};
export type Request = {
  id: number;
  revision: number;
  options: Options;
  libraryPath: string;
  filter: Filter;
};
export const defaults: Options = {
  colorPop: { ...defaultColorPop },
  colors: 8,
  totalColors: true,
  colorPriority: "balanced",
  analysisMaxPixels: 6291456,
  neutralChroma: 8,
  minClusterFraction: 0.005,
  histogramBits: 6,
  iterations: 24,
  preblurSigma: 1.5,
  smoothingColorSigma: 0,
  legacyColorPipeline: false,
  mode: "standard",
  guidanceStrength: 0.8,
  trueBlack: true,
  preserveDetails: true,
  hueforge: {
    opticalModel: "hueforge-0.9.4.3-frontlit-v1",
    firstLayerHeight: 0.16,
    lightPreset: "hueforge-default",
    layerHeight: 0.08,
    baseDepth: 0.48,
    maxDepth: 2.24,
    autoDepth: false,
    reduceShowThrough: false,
    searchEffort: "preview",
    requiredFilaments: "",
    baseFilament: "",
    highlightFilament: "",
    surfaceColorTolerance: 5,
    depthTolerance: 1,
    analysisColors: 32,
    beamWidth: 24,
    maxRuns: 0,
    meshMode: "color-match",
    meshCore: "compact-blends",
    exportWidthMm: 200,
    meshDetailMm: 0.2,
    maxPerceivedColors: 64,
    tdTransmission: 0.05,
    tdScale: 0.1,
    baseTransmissionLimit: 0.1,
  },
};
export type SurfaceView = {
  width: number;
  height: number;
  layers: number[];
  widthMm: number;
  heightMm: number;
  spacingMm: number;
  requestedSpacingMm: number;
  pixelMm: number;
  sampled: boolean;
  meanJumpMm: number;
  p95JumpMm: number;
  maxJumpMm: number;
  boundaryAreaFraction: number;
  solidifiedFraction: number;
};
export type Comparison = {
  name: string;
  source: string;
  options: Options;
  filter: Filter;
  librarySHA256?: string;
  image: string;
  quality: Result["quality"];
  colors: number;
  spools: number;
  swaps: number;
  depth: number;
  boundaryStep: number;
  rgbaSHA256: string;
};
export const emptyFilter: Filter = {
  includeUnowned: false,
  materialTypes: [],
  allowSecondary: false,
  avoidSilkMetallic: false,
  excludedIds: [],
};
