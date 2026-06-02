package decomp

// CodeAster extracts the inter-rank communication graph from a Code_Aster
// case partitioned by MEDPartitioner (CEA/EDF, ships with MEDCoupling).
//
// The workflow is driven by scripts/extract_codeaster_graph.py:
//
//   1. Run `medpartitioner --input-file=mesh.med --ndomains=N
//      --create-boundary-faces --output-file=part`. The flag instructs
//      the partitioner to persist *joints* (the same data structure
//      JointFinder.cxx and ConnectZone.cxx assemble internally) into each
//      output part_<i>.med file.
//
//   2. The script walks every part_<i>.med via h5py, reads the JOINTS
//      group, and for each (i, j) pair counts the shared nodes — the
//      number of degrees of freedom that ranks i and j must exchange.
//
//   3. The script emits an edge-list file that the present package reads
//      with the same EdgeList() function used for any solver-agnostic
//      fallback.
//
// Because medpartitioner uses the same METIS/SCOTCH pipeline Code_Aster
// runs internally (PARTITIONNEUR='PTSCOTCH' or 'METIS'), the resulting
// graph is exactly the one the running solver will exchange data over.
//
// Despite the alias to METIS() below being a thin wrapper kept for
// symmetry with OpenRadioss, the recommended path for Code_Aster is the
// Python preprocessor + decomp.EdgeList(). The METIS() variant remains
// available if a user already has a raw mesh.graph + mesh.part.N pair on
// hand (e.g. produced by manual SCOTCH invocation).
func CodeAster(graphPath, partPath string) (*Graph, error) {
	return METIS(graphPath, partPath)
}
