package decomp

// OpenRadioss extracts the inter-rank communication graph from an
// OpenRadioss case decomposed by the Starter (`-np N`).
//
// OpenRadioss has a *built-in* METIS graph dumper hidden behind a debug
// guard in starter/source/spmd/domain_decomposition/grid2mat.F:
//
//     IDB_METIS = 0       ! patched to 1 by our Docker layer
//     IF(IDB_METIS == 1) THEN
//         OPEN(99, file="input.graph"//CHLEVEL, FORM='FORMATTED', ...)
//         write(99,*) nelem, nedges, "010", ncond
//         ...
//     END IF
//
// docker/openradioss/Dockerfile flips IDB_METIS to 1 with a sed-patch
// before building the Starter; running the patched Starter produces
// `input.graph<L>` files in standard METIS adjacency format (fmt=010,
// vertex weights present, edge weights absent) — exactly what
// METIS() consumes below.
//
// To obtain the partition file (mesh.part.N) the entry-point script in
// the container runs `gpmetis input.graph0 N` after Starter exits.
//
// This is therefore a thin wrapper around METIS() pointing at the two
// artefacts produced by the patched Starter + gpmetis combo.
func OpenRadioss(graphPath, partPath string) (*Graph, error) {
	return METIS(graphPath, partPath)
}
