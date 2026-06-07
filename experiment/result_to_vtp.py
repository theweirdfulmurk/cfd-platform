#!/usr/bin/env python3
"""Convert a solver result mesh (Code_Aster MED, OpenRadioss anim→legacy VTK)
into a render-ready surface PolyData (.vtp) + stats.json for the platform viewer.

vtk.js (frontend) can only render PolyData — it has XMLPolyDataReader (.vtp) and
a legacy PolyDataReader, but NO UnstructuredGrid reader. meshio reads MED/VTK but
only writes UnstructuredGrid, so we extract a surface and hand-write the .vtp.

Pipeline per solver:
  * Code_Aster: read .rmed (MED), 3D solid (tetra/hexa) → extract the skin
    (boundary faces) → map nodal von Mises (SIEQ_NOEU) + |DEPL| onto it.
  * OpenRadioss: anim_to_vtk writes legacy UnstructuredGrid; a crash model is
    mostly SHELL elements (already a surface) → use the 2D cells directly, map
    von Mises / plastic strain.

Raw physical values are kept in the .vtp; the engineering unit shown in the UI
comes from FieldStat.scale in stats.json (Code_Aster Pa→МПа 1e-6, m→мм 1e3).

Usage:
  result_to_vtp.py --solver code_aster  result.rmed  OUTDIR
  result_to_vtp.py --solver openradioss anim.vtk      OUTDIR
  (override OpenRadioss stress unit once the model's unit system is known:
   --stress-unit ГПа --stress-scale 1)

Host deps:  pip install meshio numpy h5py    (h5py only needed for MED)
"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

import numpy as np

try:
    import meshio
except ImportError:
    sys.exit("meshio not installed — run: pip install meshio numpy h5py")


# Faces of the 3D cell types we skin (corner nodes only; quadratic cells reuse
# their corner connectivity). Each entry lists vertex-index tuples per face.
SOLID_FACES = {
    "tetra":   [(0, 1, 2), (0, 1, 3), (1, 2, 3), (0, 2, 3)],
    "tetra10": [(0, 1, 2), (0, 1, 3), (1, 2, 3), (0, 2, 3)],
    "hexahedron": [(0, 1, 2, 3), (4, 5, 6, 7), (0, 1, 5, 4),
                   (1, 2, 6, 5), (2, 3, 7, 6), (3, 0, 4, 7)],
    "wedge": [(0, 1, 2), (3, 4, 5), (0, 1, 4, 3), (1, 2, 5, 4), (2, 0, 3, 5)],
    "pyramid": [(0, 1, 2, 3), (0, 1, 4), (1, 2, 4), (2, 3, 4), (3, 0, 4)],
}
SURFACE_TYPES = {"triangle", "quad", "triangle6", "quad8", "quad9"}
CORNERS = {"triangle": 3, "triangle6": 3, "quad": 4, "quad8": 4, "quad9": 4}

# Canonical fields per solver: (canon name, label, unit, scale, name-match
# substrings, reduce). reduce: 'as_is' | 'magnitude' (vector→|v|) | 'vmis'
# (pick the von Mises component of a multi-component SIEQ array).
FIELD_SPECS = {
    "code_aster": [
        ("vonMises", "Напряжение по Мизесу", "МПа", 1e-6,
         ("sieq", "vmis", "mises"), "vmis"),
        ("displacement", "Перемещение", "мм", 1e3,
         ("depl", "displ"), "magnitude"),
    ],
    "openradioss": [
        ("vonMises", "Напряжение по Мизесу", None, None,
         ("vmis", "vonm", "mises"), "as_is"),
        ("plasticStrain", "Пластическая деформация", "—", 1.0,
         ("plas", "epsp", "pstrain"), "as_is"),
        ("displacement", "Перемещение", "мм", 1.0,
         ("displ", "depl"), "magnitude"),
    ],
}


def find_field(name_subs, point_data, cell_data_flat):
    """Locate a field by fuzzy name match. Returns (values, location) where
    location is 'point' or 'cell', or None."""
    for store, loc in ((point_data, "point"), (cell_data_flat, "cell")):
        for key, arr in store.items():
            low = key.lower()
            if any(sub in low for sub in name_subs):
                return arr, loc
    return None


def reduce_field(arr, how):
    arr = np.asarray(arr, dtype=np.float64)
    if arr.ndim == 1:
        return arr
    if how == "magnitude":
        return np.linalg.norm(arr, axis=1)
    if how == "vmis":
        # Code_Aster SIEQ_NOEU: component 0 is VMIS (von Mises). If meshio
        # already split components into separate 1-comp arrays we never reach
        # here (ndim==1 above).
        return arr[:, 0]
    return arr[:, 0]  # as_is on a multi-comp array: take the first component


def cell_to_point(values_global, faces, owners, n_points):
    """Average a per-cell value (indexed by GLOBAL cell id via `owners`) onto the
    points of each surface face it touches."""
    acc = np.zeros(n_points, dtype=np.float64)
    cnt = np.zeros(n_points, dtype=np.float64)
    for fi, face in enumerate(faces):
        v = values_global[owners[fi]]
        for p in face:
            acc[p] += v
            cnt[p] += 1.0
    cnt[cnt == 0] = 1.0
    return acc / cnt


def build_surface(mesh):
    """Return (faces, owners): the renderable surface as polygon faces (tuples of
    point indices) plus, for each face, the GLOBAL cell index that owns it (so
    cell_data — concatenated in block order — maps correctly across mixed blocks).
    The surface is the UNION of every 2D cell (shells, used as-is) AND the skin of
    every 3D solid block (boundary faces = faces used by exactly one cell). A mixed
    shell+solid mesh must render BOTH — do not short-circuit on shells, else the
    whole solid body becomes invisible."""
    faces, owners = [], []
    gci = 0  # running global cell index, aligned with concatenated cell_data
    solids = []  # (block, base_gci)
    for block in mesh.cells:
        n = len(block.data)
        if block.type in SURFACE_TYPES:
            k = CORNERS[block.type]
            for i, row in enumerate(block.data):
                faces.append(tuple(int(x) for x in row[:k]))
                owners.append(gci + i)
        elif block.type in SOLID_FACES:
            solids.append((block, gci))
        gci += n

    # Skin the solids and append to the shell faces. A boundary face appears in
    # exactly one cell; an interior (shared) face is dropped.
    seen = {}  # sorted-key -> (face, owner_gci) or None if shared
    for block, base in solids:
        face_defs = SOLID_FACES[block.type]
        for i, cell in enumerate(block.data):
            for fdef in face_defs:
                face = tuple(int(cell[j]) for j in fdef)
                key = tuple(sorted(face))
                if key in seen:
                    seen[key] = None              # interior → drop
                else:
                    seen[key] = (face, base + i)  # boundary candidate
    for v in seen.values():
        if v is not None:
            faces.append(v[0])
            owners.append(v[1])
    return faces, owners


def write_vtp(path, points, faces, point_fields):
    """Hand-write an ASCII XML PolyData (.vtp) that vtk.js XMLPolyDataReader reads."""
    np_pts = len(points)
    conn, offsets, off = [], [], 0
    for f in faces:
        conn.extend(f)
        off += len(f)
        offsets.append(off)

    def fmt(a):
        return " ".join(f"{v:.6g}" for v in np.asarray(a).ravel())

    scalars = next(iter(point_fields), "")
    lines = [
        '<?xml version="1.0"?>',
        '<VTKFile type="PolyData" version="1.0" byte_order="LittleEndian">',
        "  <PolyData>",
        f'    <Piece NumberOfPoints="{np_pts}" NumberOfVerts="0" '
        f'NumberOfLines="0" NumberOfStrips="0" NumberOfPolys="{len(faces)}">',
        "      <Points>",
        '        <DataArray type="Float32" NumberOfComponents="3" format="ascii">',
        "          " + fmt(points),
        "        </DataArray>",
        "      </Points>",
        f'      <PointData Scalars="{scalars}">',
    ]
    for name, vals in point_fields.items():
        lines += [
            f'        <DataArray type="Float32" Name="{name}" '
            f'NumberOfComponents="1" format="ascii">',
            "          " + fmt(vals),
            "        </DataArray>",
        ]
    lines += [
        "      </PointData>",
        "      <Polys>",
        '        <DataArray type="Int64" Name="connectivity" format="ascii">',
        "          " + " ".join(str(int(c)) for c in conn),
        "        </DataArray>",
        '        <DataArray type="Int64" Name="offsets" format="ascii">',
        "          " + " ".join(str(int(o)) for o in offsets),
        "        </DataArray>",
        "      </Polys>",
        "    </Piece>",
        "  </PolyData>",
        "</VTKFile>",
    ]
    Path(path).write_text("\n".join(lines))


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("input")
    ap.add_argument("outdir")
    ap.add_argument("--solver", required=True, choices=list(FIELD_SPECS))
    ap.add_argument("--stress-unit", default="МПа",
                    help="OpenRadioss stress unit (model-dependent; set from the "
                         "model's unit system — mm-ms-kg→ГПа, mm-s-tonne→МПа)")
    ap.add_argument("--stress-scale", type=float, default=1.0,
                    help="OpenRadioss stress display multiplier")
    args = ap.parse_args()

    out = Path(args.outdir)
    out.mkdir(parents=True, exist_ok=True)

    mesh = meshio.read(args.input)
    points = np.asarray(mesh.points, dtype=np.float64)
    if points.shape[1] == 2:
        points = np.column_stack([points, np.zeros(len(points))])

    # Flatten cell_data across blocks (one value per global cell, in block order)
    # is order-sensitive; we only need cell→point averaging over the surface, so
    # collapse to {name: concatenated array} aligned with build order is enough
    # for shells (one block) and skinned solids (we re-derive per-face below).
    point_data = {k: np.asarray(v) for k, v in mesh.point_data.items()}
    cell_data_flat = {}
    for name, blocks in mesh.cell_data.items():
        cell_data_flat[name] = np.concatenate([np.asarray(b) for b in blocks])

    faces, owners = build_surface(mesh)
    if not faces:
        sys.exit("no renderable surface (no 2D cells and no skinnable solids)")
    print(f"[vtp] {len(faces)} surface faces, {len(points)} points")

    specs = FIELD_SPECS[args.solver]
    point_fields, stats_fields = {}, []
    for canon, label, unit, scale, subs, how in specs:
        found = find_field(subs, point_data, cell_data_flat)
        if not found:
            print(f"[vtp] field '{canon}' not found (tried {subs}) — skipping")
            continue
        raw, loc = found
        vals = reduce_field(raw, how)
        if loc == "cell":
            vals = cell_to_point(np.asarray(vals, dtype=np.float64), faces, owners, len(points))
        point_fields[canon] = vals.astype(np.float32)
        u = args.stress_unit if (args.solver == "openradioss" and canon == "vonMises") else unit
        s = args.stress_scale if (args.solver == "openradioss" and canon == "vonMises") else scale
        stats_fields.append({"name": canon, "label": label,
                             "unit": u or "—", "scale": s if s is not None else 1.0})

    if not point_fields:
        sys.exit("no known fields matched — cannot colour the surface")

    # Compact to surface-referenced nodes only. vtk.js getRange() (legend) and
    # fieldSummary (CSV) scan EVERY point in the array, so keeping interior solid
    # nodes makes the legend max/mean reflect buried extrema not visible on the
    # surface. Drop orphan nodes and remap the face connectivity to the subset.
    used = sorted({p for f in faces for p in f})
    remap = {old: new for new, old in enumerate(used)}
    faces = [tuple(remap[p] for p in f) for f in faces]
    points = points[used]
    for name in list(point_fields):
        point_fields[name] = point_fields[name][used]

    # Sanitize non-finite values: %.6g would emit literal nan/inf tokens, and a
    # single NaN makes vtk.js getRange() return [NaN,NaN], collapsing the whole
    # colour map + legend. Map NaN/inf to 0 (a blown-up node shown as 0 is no
    # worse than as a poison value).
    points = np.nan_to_num(points, nan=0.0, posinf=0.0, neginf=0.0)
    for name in list(point_fields):
        point_fields[name] = np.nan_to_num(
            point_fields[name], nan=0.0, posinf=0.0, neginf=0.0).astype(np.float32)
    print(f"[vtp] compacted to {len(points)} surface nodes")

    write_vtp(out / "surface.vtp", points, faces, point_fields)
    stats = {"surface": Path(args.input).stem, "cells": len(faces), "fields": stats_fields}
    (out / "stats.json").write_text(json.dumps(stats, ensure_ascii=False, indent=2))
    print(f"[vtp] wrote {out/'surface.vtp'} + stats.json: "
          f"{[f['name'] for f in stats_fields]}")


if __name__ == "__main__":
    main()
