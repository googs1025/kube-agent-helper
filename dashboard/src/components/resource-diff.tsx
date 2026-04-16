"use client";

import ReactDiffViewer, { DiffMethod } from "react-diff-viewer-continued";
import { useTheme } from "@/theme/context";

interface Props {
  before: string; // raw YAML or JSON text
  after: string;  // raw YAML or JSON text
}

export function ResourceDiff({ before, after }: Props) {
  const { theme } = useTheme();
  return (
    <ReactDiffViewer
      oldValue={before}
      newValue={after}
      splitView
      useDarkTheme={theme === "dark"}
      compareMethod={DiffMethod.LINES}
    />
  );
}
