// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Geometry for cosmetic edge routing. A waypoint is stored relative to the
// source->target line (t = fraction along it, off = signed perpendicular offset
// in graph units), so it follows when either endpoint moves. These pure helpers
// convert between that relative form and absolute canvas coordinates; ElkEdge
// uses them to draw the route and to turn a drag back into a stored waypoint.

import type { EdgeWaypoint } from "../model";

export interface Point {
  x: number;
  y: number;
}

// The source->target vector plus its unit normal. Degenerate (coincident
// endpoints, e.g. a self-loop mid-drag) falls back to a unit x-axis so the math
// stays finite instead of dividing by zero.
function frame(source: Point, target: Point) {
  const dx = target.x - source.x;
  const dy = target.y - source.y;
  const len = Math.hypot(dx, dy);
  if (len === 0) return { dx: 1, dy: 0, nx: 0, ny: 1, len: 1 };
  return { dx, dy, nx: -dy / len, ny: dx / len, len };
}

// Absolute canvas point for a relative waypoint, given the current endpoints.
export function waypointToAbsolute(
  source: Point,
  target: Point,
  wp: EdgeWaypoint,
): Point {
  const f = frame(source, target);
  return {
    x: source.x + wp.t * f.dx + wp.off * f.nx,
    y: source.y + wp.t * f.dy + wp.off * f.ny,
  };
}

// Relative waypoint for an absolute point, given the current endpoints. Inverse
// of waypointToAbsolute: t is the projection onto the source->target line, off
// the signed distance from it along the unit normal.
export function absoluteToWaypoint(
  source: Point,
  target: Point,
  p: Point,
): EdgeWaypoint {
  const f = frame(source, target);
  const rx = p.x - source.x;
  const ry = p.y - source.y;
  const t = (rx * f.dx + ry * f.dy) / (f.len * f.len);
  const off = rx * f.nx + ry * f.ny;
  return { t, off };
}

// A right-angle route from source to target that bends through `through`, so dragging
// `through` moves the relation freely in both axes (steer it around nodes) while it
// stays clean straight segments with square corners. Each leg turns along its longer
// span first, keeping the elbows tidy; a leg whose corner coincides with an endpoint
// collapses to a straight segment.
export function orthogonalRoute(source: Point, target: Point, through: Point): Point[] {
  const corner = (from: Point, to: Point): Point =>
    Math.abs(to.x - from.x) >= Math.abs(to.y - from.y)
      ? { x: to.x, y: from.y }
      : { x: from.x, y: to.y };
  return [source, corner(source, through), through, corner(through, target), target];
}

function distance(a: Point, b: Point): number {
  return Math.hypot(b.x - a.x, b.y - a.y);
}

// A point `r` units from `from` toward `to` (or `from` itself when they coincide).
function toward(from: Point, to: Point, r: number): Point {
  const len = distance(from, to);
  if (len === 0) return from;
  return { x: from.x + ((to.x - from.x) / len) * r, y: from.y + ((to.y - from.y) / len) * r };
}

// An SVG path over an orthogonal polyline whose corners are rounded to `radius`, so the
// right-angle bends read as clean arcs. Consecutive duplicate points (a zero-length
// trunk when it sits on an endpoint) collapse so no spurious corner is drawn.
export function roundedPath(points: Point[], radius: number): string {
  const pts: Point[] = [];
  for (const p of points) {
    const last = pts[pts.length - 1];
    if (!last || distance(last, p) > 0.01) pts.push(p);
  }
  if (pts.length < 2) return "";
  let d = `M ${pts[0].x} ${pts[0].y}`;
  for (let i = 1; i < pts.length - 1; i++) {
    const prev = pts[i - 1];
    const cur = pts[i];
    const next = pts[i + 1];
    const a = toward(cur, prev, Math.min(radius, distance(cur, prev) / 2));
    const b = toward(cur, next, Math.min(radius, distance(cur, next) / 2));
    d += ` L ${a.x} ${a.y} Q ${cur.x} ${cur.y} ${b.x} ${b.y}`;
  }
  const end = pts[pts.length - 1];
  d += ` L ${end.x} ${end.y}`;
  return d;
}
