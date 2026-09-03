export function projectContextMenuPosition({
  height,
  margin = 8,
  viewportHeight,
  viewportWidth,
  width,
  x,
  y,
}) {
  return {
    left: Math.max(margin, Math.min(x, viewportWidth - width - margin)),
    top: Math.max(margin, Math.min(y, viewportHeight - height - margin)),
  }
}
