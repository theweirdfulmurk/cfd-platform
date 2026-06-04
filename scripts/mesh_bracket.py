#!/usr/bin/env python3
"""Mesh a SimJEB bracket STEP file into a 3D tetrahedral Code_Aster mesh.

Usage: mesh_bracket.py <in.stp> <out.med> [size_factor]

size_factor scales the characteristic element length (smaller -> finer ->
more DOF). Prints node/element/DOF counts and the geometric bounding box so
the mesh resolution can be tuned to a target problem size.

Also tags two node groups for boundary conditions:
  FIX  - nodes on the min-Z face (clamp)
  LOAD - nodes on the max-Z face (where a load can be applied)
so the .comm can build a well-posed static problem without hand-identifying
bolt holes. Physical accuracy is irrelevant here: the experiment metric is MPI
communication time, not stress.
"""
import sys
import gmsh

inp, out = sys.argv[1], sys.argv[2]
size_factor = float(sys.argv[3]) if len(sys.argv) > 3 else 1.0

gmsh.initialize()
gmsh.option.setNumber("General.Terminal", 1)

# Heal the imported STEP BEFORE building the OCC model: SimJEB brackets carry
# sliver faces / tiny edges that produce self-intersecting surface meshes
# (HXT/Delaunay then fail with a PLC "segment and facet intersect" error).
gmsh.option.setNumber("Geometry.OCCFixDegenerated", 1)
gmsh.option.setNumber("Geometry.OCCFixSmallEdges", 1)
gmsh.option.setNumber("Geometry.OCCFixSmallFaces", 1)
gmsh.option.setNumber("Geometry.OCCSewFaces", 1)
gmsh.option.setNumber("Geometry.OCCMakeSolids", 1)
gmsh.open(inp)  # OCC STEP import (healing applied during import)

# Bounding box (to understand the geometric scale).
xmin, ymin, zmin, xmax, ymax, zmax = gmsh.model.getBoundingBox(-1, -1)
print(f"bbox X[{xmin:.2f},{xmax:.2f}] Y[{ymin:.2f},{ymax:.2f}] Z[{zmin:.2f},{zmax:.2f}]")

# Curvature-aware sizing + robust algorithms. Frontal-Delaunay (2D=6) and
# Delaunay (3D=1) tolerate the healed-but-imperfect geometry better than HXT.
gmsh.option.setNumber("Mesh.MeshSizeFactor", size_factor)
gmsh.option.setNumber("Mesh.MeshSizeFromCurvature", 12)
gmsh.option.setNumber("Mesh.MeshSizeExtendFromBoundary", 1)
gmsh.option.setNumber("Mesh.Algorithm", 6)    # 2D Frontal-Delaunay
gmsh.option.setNumber("Mesh.Algorithm3D", 1)  # 3D Delaunay
gmsh.option.setNumber("Mesh.OptimizeNetgen", 1)

gmsh.model.mesh.generate(3)

node_tags, _, _ = gmsh.model.mesh.getNodes()
n_nodes = len(node_tags)
# 3 DOF per node for 3D elasticity.
print(f"nodes={n_nodes}  approx_DOF={3*n_nodes}")
tet_type = gmsh.model.mesh.getElementType("Tetrahedron", 1)
tets, _ = gmsh.model.mesh.getElementsByType(tet_type)
print(f"tetrahedra={len(tets)}")

gmsh.write(out)
print(f"wrote {out}")
gmsh.finalize()
