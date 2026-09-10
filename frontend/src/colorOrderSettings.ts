import type { ColorOrderReport } from "./types.ts";

export function orderedColorGroups(
  groups: ColorOrderReport["groups"],
  order: string,
) {
  const keys = order.split(",");
  return [...groups].sort((a, b) => {
    const rank = (key: string) =>
      keys.includes(key) ? keys.indexOf(key) : keys.length;
    return rank(a.key) - rank(b.key);
  });
}

export function moveColorGroup(
  keys: string[],
  from: string,
  to: string,
): string[] {
  const start = keys.indexOf(from),
    end = keys.indexOf(to);
  if (start < 0 || end < 0 || start === end) return [...keys];
  const next = [...keys];
  next.splice(start, 1);
  next.splice(end, 0, from);
  return next;
}
