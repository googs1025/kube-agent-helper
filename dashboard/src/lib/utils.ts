import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"
import jsYaml from "js-yaml";
import * as jsonpatch from "fast-json-patch";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * Apply a Fix's patch client-side to the beforeSnapshot for preview.
 * Returns a pretty-printed JSON string, or empty string on failure.
 */
export function computeAfter(
  beforeSnapshot: string,
  patchType: string,
  patchContent: string,
): string {
  if (!beforeSnapshot) return "";
  let beforeObj: unknown;
  try {
    const decoded = atob(beforeSnapshot);
    try {
      beforeObj = JSON.parse(decoded);
    } catch {
      beforeObj = jsYaml.load(decoded);
    }
  } catch {
    return "";
  }

  let afterObj: unknown;
  try {
    if (patchType === "json-patch") {
      const ops = JSON.parse(patchContent);
      afterObj = jsonpatch.applyPatch(
        JSON.parse(JSON.stringify(beforeObj)),
        ops,
      ).newDocument;
    } else {
      // strategic-merge: deep-merge patch into before
      const patchObj = JSON.parse(patchContent);
      afterObj = deepMerge(
        JSON.parse(JSON.stringify(beforeObj)),
        patchObj,
      );
    }
  } catch {
    return "";
  }

  return JSON.stringify(afterObj, null, 2);
}

export function decodeBefore(beforeSnapshot: string): string {
  if (!beforeSnapshot) return "";
  try {
    const decoded = atob(beforeSnapshot);
    try {
      return JSON.stringify(JSON.parse(decoded), null, 2);
    } catch {
      return decoded;
    }
  } catch {
    return "";
  }
}

function deepMerge<T extends Record<string, unknown>>(a: T, b: T): T {
  for (const key of Object.keys(b)) {
    const aVal = (a as Record<string, unknown>)[key];
    const bVal = (b as Record<string, unknown>)[key];
    if (
      aVal && bVal &&
      typeof aVal === "object" && typeof bVal === "object" &&
      !Array.isArray(aVal) && !Array.isArray(bVal)
    ) {
      (a as Record<string, unknown>)[key] = deepMerge(
        aVal as Record<string, unknown>,
        bVal as Record<string, unknown>,
      );
    } else {
      (a as Record<string, unknown>)[key] = bVal;
    }
  }
  return a;
}
