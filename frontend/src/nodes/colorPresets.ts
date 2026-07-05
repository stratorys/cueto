// Shared color presets for the node color popover (fill + border). Fill presets
// are soft tints; "None" clears the fill back to the default. Border presets are
// saturated; "None" is a transparent (invisible) border.
export const FILL_PRESETS: { title: string; value: string | undefined }[] = [
  { title: "None", value: undefined },
  { title: "Slate", value: "#f1f5f9" },
  { title: "Red", value: "#fee2e2" },
  { title: "Amber", value: "#fef3c7" },
  { title: "Green", value: "#dcfce7" },
  { title: "Blue", value: "#dbeafe" },
  { title: "Violet", value: "#ede9fe" },
];

export const STROKE_PRESETS: { title: string; value: string }[] = [
  { title: "None", value: "transparent" },
  { title: "Slate", value: "#94a3b8" },
  { title: "Red", value: "#ef4444" },
  { title: "Amber", value: "#f59e0b" },
  { title: "Green", value: "#22c55e" },
  { title: "Blue", value: "#3b82f6" },
  { title: "Violet", value: "#8b5cf6" },
];
