#!/usr/bin/env python3
"""Build the communication graph F(i,j) for a Code_Aster .med mesh — without
MEDCoupling.

The originally-designed route (medpartitioner + MEDLoader joints, see
extract_codeaster_graph.py) needs the MEDCoupling toolkit, which is NOT built
into our Code_Aster image (only the low-level MED file library is). This
replacement uses only what the image ships:

    * the low-level `med` Python binding (reads .med connectivity)
    * `mpmetis` from /aster/metis/bin (partitions the element mesh)
    * numpy

Workflow:

    1. Open the .med mesh, read the top-dimension cell connectivity via the
       `med` binding (buffers are med.MEDINT C-arrays, not numpy).
    2. Write a METIS mesh file (one line of 1-based node ids per element) and
       run `mpmetis <mesh> <ndomains>` -> <mesh>.epart.<n> (element->part).
    3. F(i,j) = number of mesh nodes shared between subdomains i and j: for
       every node, collect the partitions of its incident elements; a node
       touching parts {p,q,...} adds 1 to F for each unordered pair. This is
       the same shared-node-count semantics as the medpartitioner joints.
    4. Emit an edge-list `pkg/decomp.EdgeList()` reads natively:

           # comment
           N
           i j weight

Usage:
    extract_codeaster_graph_metis.py --input mesh.med --ndomains 8 --output graph.edgelist

NB: the Code_Aster image ships Python 3.6, so this stays 3.6-compatible
(no `from __future__ import annotations`, no PEP 585 `list[int]` generics).
"""
import argparse
import itertools
import subprocess
import sys
import tempfile
from pathlib import Path

import med

# (name, geotype, nodes-per-element, topological dimension). Only the highest
# dimension present is partitioned; lower-dim cells are mesh boundaries.
GEOTYPES = [
    ("SEG2", med.MED_SEG2, 2, 1),
    ("SEG3", med.MED_SEG3, 3, 1),
    ("TRIA3", med.MED_TRIA3, 3, 2),
    ("TRIA6", med.MED_TRIA6, 6, 2),
    ("QUAD4", med.MED_QUAD4, 4, 2),
    ("QUAD8", med.MED_QUAD8, 8, 2),
    ("QUAD9", med.MED_QUAD9, 9, 2),
    ("TETRA4", med.MED_TETRA4, 4, 3),
    ("TETRA10", med.MED_TETRA10, 10, 3),
    ("PYRA5", med.MED_PYRA5, 5, 3),
    ("PENTA6", med.MED_PENTA6, 6, 3),
    ("PENTA15", med.MED_PENTA15, 15, 3),
    ("HEXA8", med.MED_HEXA8, 8, 3),
    ("HEXA20", med.MED_HEXA20, 20, 3),
]


def mesh_name(fid) -> str:
    info = med.MEDmeshInfo(fid, 1)
    return info[0] if isinstance(info, (list, tuple)) else info


def n_cells(fid, name, gt) -> int:
    r = med.MEDmeshnEntity(fid, name, med.MED_NO_DT, med.MED_NO_IT,
                           med.MED_CELL, gt, med.MED_CONNECTIVITY, med.MED_NODAL)
    return r[0] if isinstance(r, (list, tuple)) else r


def read_conn(fid, name, gt, ncell, npe):
    buf = med.MEDINT(ncell * npe)
    med.MEDmeshElementConnectivityRd(fid, name, med.MED_NO_DT, med.MED_NO_IT,
                                     med.MED_CELL, gt, med.MED_NODAL,
                                     med.MED_FULL_INTERLACE, buf)
    return [buf[i] for i in range(ncell * npe)]


def load_elements(med_path: Path):
    """Return (elements, nnodes): elements is a list of node-id tuples for the
    highest-dimension cells, node ids 1-based as stored in the .med file."""
    fid = med.MEDfileOpen(str(med_path), med.MED_ACC_RDONLY)
    name = mesh_name(fid)
    present = []  # (npe, dim, flat_conn, ncell)
    for _label, gt, npe, dim in GEOTYPES:
        ncell = n_cells(fid, name, gt)
        if ncell and ncell > 0:
            present.append((npe, dim, read_conn(fid, name, gt, ncell, npe), ncell))
    if not present:
        sys.exit("error: no recognised cells in mesh")
    topdim = max(p[1] for p in present)
    elements = []
    maxnode = 0
    for npe, dim, flat, ncell in present:
        if dim != topdim:
            continue
        for e in range(ncell):
            nodes = flat[e * npe:(e + 1) * npe]
            elements.append(tuple(nodes))
            maxnode = max(maxnode, *nodes)
    return elements, maxnode


def partition(elements, ndomains, workdir):
    """Write a METIS mesh file, run mpmetis, return element->partition list."""
    meshfile = workdir / "ca.mesh"
    with meshfile.open("w") as f:
        f.write(f"{len(elements)}\n")
        for nodes in elements:
            f.write(" ".join(str(n) for n in nodes) + "\n")
    subprocess.run(["mpmetis", str(meshfile), str(ndomains)],
                   check=True, stdout=subprocess.DEVNULL)
    epart_file = workdir / f"ca.mesh.epart.{ndomains}"
    return [int(x) for x in epart_file.read_text().split()]


def shared_node_graph(elements, epart, ndomains):
    """F(i,j) = count of nodes shared between subdomains i and j."""
    node_parts = {}
    for e, nodes in enumerate(elements):
        p = epart[e]
        for n in nodes:
            node_parts.setdefault(n, set()).add(p)
    weights = {}
    for parts in node_parts.values():
        if len(parts) < 2:
            continue
        for i, j in itertools.combinations(sorted(parts), 2):
            weights[(i, j)] = weights.get((i, j), 0) + 1
    return weights


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--input", required=True, type=Path)
    ap.add_argument("--ndomains", required=True, type=int)
    ap.add_argument("--output", type=Path)
    args = ap.parse_args()

    elements, _nnodes = load_elements(args.input)
    print(f"[extract] {len(elements)} top-dim cells, ndomains={args.ndomains}",
          file=sys.stderr)
    with tempfile.TemporaryDirectory() as td:
        epart = partition(elements, args.ndomains, Path(td))
    weights = shared_node_graph(elements, epart, args.ndomains)

    out = sys.stdout if not args.output else args.output.open("w")
    out.write("# F-graph from Code_Aster mesh (med + mpmetis)\n")
    out.write("# generated by extract_codeaster_graph_metis.py\n")
    out.write(f"{args.ndomains}\n")
    for (i, j), w in sorted(weights.items()):
        out.write(f"{i} {j} {w}\n")
    if args.output:
        out.close()


if __name__ == "__main__":
    main()
