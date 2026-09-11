import type { Filter, Library } from "./types";
export type Measurement = {
  td: number;
  color: string;
  serial: string;
  capturedAt: string;
};
export type Reading = Measurement & { id: number; raw: string };
export type FilamentEntry = {
  index: number;
  uuid: string;
  brand: string;
  name: string;
  material: string;
  color: string;
  secondaryColor: string;
  td: number;
  owned: boolean;
  tags: string[];
  sourceURL: string;
  measurement?: Measurement;
};
export type Catalog = {
  path: string;
  sha256: string;
  managed: boolean;
  entries: FilamentEntry[];
  library: Library;
  filter: Filter;
  warning: string;
  keyChanges?: Record<string, string>;
};
export type DevicePort = { name: string; serial: string; product: string };
export type DeviceState = {
  connected: boolean;
  port: DevicePort;
  status: string;
  error: string;
  identity: string;
  version: string;
  latest?: Reading;
  readings: Reading[];
  messages: string[];
  display: { text: string; x: number; y: number }[];
  busy: boolean;
};
export type Profile = {
  id: string;
  brand: string;
  material: string;
  type: string;
  color: string;
  hex: string;
  secondaryHex: string;
  td: number;
  url: string;
};
export type ProfileResults = {
  profiles: Profile[];
};
export function profileDetailURL(value: string): string {
  value = value.trim();
  if (/^[1-9][0-9]{0,9}$/.test(value))
    return `https://3dfilamentprofiles.com/filament/details/${value}`;
  if (
    /^https:\/\/3dfilamentprofiles\.com\/filament\/details\/[1-9][0-9]{0,9}$/.test(
      value,
    )
  )
    return value;
  return "";
}
export type FirmwarePlan = {
  sha256: string;
  files: { name: string; size: number }[];
  prerequisites: Record<string, string>;
};
export type DeviceResponse = {
  state: DeviceState;
  text: string;
  settings?: Record<string, string>;
  files?: string[];
  firmware?: { latestVersion: string; releaseNotes: string[] };
  plan?: FirmwarePlan;
  path?: string;
  recovery?: RecoveryPlan;
};
export type RecoveryPlan = {
  path: string;
  sha256: string;
  size: number;
  blocks: number;
  volume: string;
  board: string;
  serial: string;
  backup: string;
};
export function newFilament(): FilamentEntry {
  return {
    index: -1,
    uuid: "",
    brand: "",
    name: "",
    material: "PLA",
    color: "#808080",
    secondaryColor: "",
    td: 0,
    owned: true,
    tags: [],
    sourceURL: "",
  };
}
export function measuredEntry(
  entry: FilamentEntry,
  reading: Measurement,
  updateColor: boolean,
): FilamentEntry {
  return {
    ...entry,
    td: reading.td,
    color: updateColor ? reading.color : entry.color,
    measurement: {
      td: reading.td,
      color: reading.color,
      serial: reading.serial,
      capturedAt: reading.capturedAt,
    },
  };
}
export function profileEntry(profile: Profile): FilamentEntry {
  return {
    ...newFilament(),
    brand: profile.brand,
    name: profile.color,
    material: [profile.material, profile.type === "Basic" ? "" : profile.type]
      .filter(Boolean)
      .join(" "),
    color: profile.hex,
    secondaryColor: profile.secondaryHex ?? "",
    td: profile.td || 0,
    sourceURL: profile.url,
  };
}
export function remapFilamentKeys(
  keys: string | undefined,
  changes: Record<string, string>,
): string {
  return (keys ?? "")
    .split(",")
    .map((k) => k.trim())
    .map((k) => changes[k] ?? k)
    .filter(Boolean)
    .join(",");
}
