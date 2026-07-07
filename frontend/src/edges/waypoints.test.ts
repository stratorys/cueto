// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";
import { absoluteToWaypoint, orthogonalRoute, roundedPath, waypointToAbsolute } from "./waypoints";

const s = { x: 0, y: 0 };
const t = { x: 100, y: 0 };

describe("waypoint geometry", () => {
  it("places a zero-offset midpoint on the source->target line", () => {
    expect(waypointToAbsolute(s, t, { t: 0.5, off: 0 })).toEqual({ x: 50, y: 0 });
  });

  it("offsets perpendicular to the line", () => {
    expect(waypointToAbsolute(s, t, { t: 0.5, off: 20 })).toEqual({ x: 50, y: 20 });
  });

  it("round-trips an absolute point back to the same relative waypoint", () => {
    const wp = absoluteToWaypoint(s, t, { x: 30, y: -15 });
    expect(wp.t).toBeCloseTo(0.3);
    expect(wp.off).toBeCloseTo(-15);
    expect(waypointToAbsolute(s, t, wp)).toEqual({ x: 30, y: -15 });
  });

  it("tracks the offset when the endpoints move", () => {
    const wp = absoluteToWaypoint(s, t, { x: 50, y: 20 });
    const moved = waypointToAbsolute({ x: 0, y: 0 }, { x: 0, y: 100 }, wp);
    expect(moved.x).toBeCloseTo(-20);
    expect(moved.y).toBeCloseTo(50);
  });
});

describe("orthogonalRoute", () => {
  it("bends through the dragged point in both axes with right-angle corners", () => {
    // Both legs turn along their longer span first, so the path passes through `through`.
    const pts = orthogonalRoute(s, t, { x: 70, y: 25 });
    expect(pts).toEqual([
      { x: 0, y: 0 },
      { x: 70, y: 0 },
      { x: 70, y: 25 },
      { x: 100, y: 25 },
      { x: 100, y: 0 },
    ]);
    // The dragged point is on the path, so moving it steers the relation freely.
    expect(pts).toContainEqual({ x: 70, y: 25 });
  });

  it("keeps every segment axis-aligned (pure horizontal or vertical)", () => {
    const pts = orthogonalRoute(s, { x: 40, y: 120 }, { x: 90, y: 30 });
    for (let i = 1; i < pts.length; i++) {
      const axisAligned = pts[i].x === pts[i - 1].x || pts[i].y === pts[i - 1].y;
      expect(axisAligned).toBe(true);
    }
  });
});

describe("roundedPath", () => {
  it("collapses zero-length segments so no spurious corner is drawn", () => {
    // A degenerate trunk (duplicate points) reduces to a straight line, not a kink.
    expect(roundedPath([s, s, t], 10)).toBe("M 0 0 L 100 0");
  });

  it("rounds an interior corner into an arc (L into a quadratic and out)", () => {
    const d = roundedPath(
      [
        { x: 0, y: 0 },
        { x: 50, y: 0 },
        { x: 50, y: 50 },
      ],
      10,
    );
    expect(d).toContain("Q 50 0");
    expect(d.startsWith("M 0 0")).toBe(true);
    expect(d.trimEnd().endsWith("50 50")).toBe(true);
  });
});
