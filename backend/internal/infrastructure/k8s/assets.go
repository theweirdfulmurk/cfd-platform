package k8s

import _ "embed"

// codeasterExtractScript is the Code_Aster F-graph extractor, embedded from
// assets/extract_codeaster_graph_metis.py. The Code_Aster image lacks
// MEDCoupling/medpartitioner, so the extraction Job writes this script to
// /tmp at run time (see extractionCommand) instead of calling a baked-in one.
//
//go:embed assets/extract_codeaster_graph_metis.py
var codeasterExtractScript string
