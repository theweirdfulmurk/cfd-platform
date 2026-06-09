package k8s

import (
	"context"
	"fmt"

	"github.com/theweirdfulmurk/cfd-platform/internal/domain"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// MPIJobGVR is the GroupVersionResource for kubeflow/mpi-operator v2beta1
// MPIJob custom resource. We use the dynamic client to avoid pulling the
// mpi-operator types as a hard dependency — the CRD must be installed in
// the cluster (see k8s/mpi-operator.yaml).
var MPIJobGVR = schema.GroupVersionResource{
	Group:    "kubeflow.org",
	Version:  "v2beta1",
	Resource: "mpijobs",
}

// LabelAlgorithm mirrors the constant in scheduler/plugin/plugin.go —
// the extender reads this off the Pod to decide which placement strategy
// to apply (random / greedy / mueller-merbach). The value comes from
// Simulation.Algorithm, set by the HTTP handler.
const LabelAlgorithm = "scheduler.cfd-platform/algorithm"

// mpiPLib is the LD_PRELOAD profiling library baked into every solver image
// (see docker/*/Dockerfile). Loading it into each rank makes mpiP emit a
// report at MPI_Finalize; the report directory is set via MPIP="-f <dir>".
const mpiPLib = "/opt/mpiP/lib/libmpiP.so"

// solverImage returns the container image for a given solver type. Images
// are produced by the GH Actions workflow in .github/workflows/build-images.yml
// and pushed to GHCR.
func solverImage(t domain.SimulationType) string {
	switch t {
	case domain.SimTypeOpenFOAM:
		return "ghcr.io/theweirdfulmurk/cfd-platform-openfoam:latest"
	case domain.SimTypeOpenRadioss:
		return "ghcr.io/theweirdfulmurk/cfd-platform-openradioss:latest"
	case domain.SimTypeCodeAster:
		return "ghcr.io/theweirdfulmurk/cfd-platform-codeaster:latest"
	}
	return ""
}

// solverCommand returns the launcher command for the MPIJob *run* phase,
// assuming extractionCommand() already produced the decomposed mesh
// (processor*/) and the F-graph edgelist in /scheduler-graphs/<id>.edgelist.
//
// Each rank is profiled with mpiP: LD_PRELOAD pulls in libmpiP.so and
// MPIP="-f /results/<simID>" directs the report into the shared results PVC.
// Both vars are exported to every rank with `mpirun -x`, so the report that
// rank 0 writes at MPI_Finalize lands at /results/<simID>/*.mpiP — exactly
// where experiment/run_benchmark.py:fetch_mpip() looks for it. The trailing
// cp is a fallback for the case where mpiP ignores -f and writes to rank 0's
// cwd (the case dir, also on a shared PVC).
//
// Without this LD_PRELOAD no .mpiP report is produced and the experiment's
// primary metric (MPI time) cannot be measured.
func solverCommand(t domain.SimulationType, simID, configPath string, np int) []string {
	caseDir := "/pvc/simulations/" + configPath
	resultsDir := "/results/" + simID

	var run string
	switch t {
	case domain.SimTypeOpenFOAM:
		run = fmt.Sprintf("mpirun --allow-run-as-root --mca routed direct --mca plm_rsh_no_tree_spawn 1 --mca oob_tcp_if_include eth0 --mca btl_tcp_if_include eth0 -x LD_PRELOAD -x MPIP -x PATH -x LD_LIBRARY_PATH -np %d bash -lc 'cd %s && simpleFoam -parallel'", np, caseDir)
	case domain.SimTypeOpenRadioss:
		// Each rank runs in a POD-LOCAL scratch dir (/tmp/rad) with the case
		// files symlinked in, NOT directly in the shared PVC case dir. The
		// OpenRadioss engine writes per-rank scratch temp files (NN_root_*.tmp)
		// into its CWD and reads them straight back; on the shared kind hostPath
		// PVC those reads come back empty across pods ("freeform.F: End of file"),
		// even though the very same 16-rank job runs fine with a local CWD. mpiP
		// still lands in /results via the absolute MPIP path, and the engine
		// reads restart/deck through the symlinks. (simpleFoam/Code_Aster don't
		// hit this — they don't do the create-then-read scratch dance.)
		run = fmt.Sprintf("mpirun --allow-run-as-root --mca routed direct --mca plm_rsh_no_tree_spawn 1 --mca oob_tcp_if_include eth0 --mca btl_tcp_if_include eth0 -x PATH -x LD_LIBRARY_PATH -np %d bash -lc 'rm -rf /tmp/rad && mkdir -p /tmp/rad && cd /tmp/rad && ln -sf %s/* . && engine_linux64_gf_ompi -input *.rad'", np, caseDir)
	case domain.SimTypeCodeAster:
		// code_aster is launched by running the command file directly
		// (`python3 fort.1`), the same invocation the working manual run used —
		// NOT via as_run, which does its OWN local mpiexec and would not
		// distribute ranks across the kubeflow worker pods (the whole point of
		// the experiment). The MPIJob mpirun places one rank per worker pod;
		// each runs in a POD-LOCAL, PER-RANK dir (/tmp/ca/proc.$rank) so the
		// per-rank JEVEUX base/fort.* cannot collide — both across pods on the
		// shared PVC AND between ranks that land in the same pod (np > #pods or
		// local runs), where a shared /tmp/ca + rm -rf/mkdir would race. Each
		// rank only touches its own subdir; mkdir -p creates the shared parent
		// idempotently. mpiP is NOT preloaded: like the
		// OpenRadioss Fortran engine it corrupts MPI_Comm_size (Fortran/C PMPI
		// name-mangling), making code_aster see the wrong process count.
		// We do NOT pass --mca orte_keep_fqdn_hostnames: the MPI Operator
		// hostfile lists `<pod>.<job>.<ns>.svc` (no .cluster.local), so keeping
		// the FQDN makes ORTE try to resolve a partial name that has no DNS
		// record — daemon launch fails. Letting ORTE strip to the short pod
		// hostname resolves via the pod resolv.conf search domain, exactly like
		// the working OpenFOAM/OpenRadioss jobs. Fixture: study.comm + mesh.med;
		// the bootstrap imports are prepended (legacy test comms omit them) and
		// the mesh is exposed on unit 20 (fort.20).
		//
		// We invoke the launcher by ABSOLUTE path /aster/openmpi/bin/mpirun
		// (our OpenMPI 4.1.6). In this image gmsh/grace drag in apt's OpenMPI
		// 2.1.1 and profile.sh's PATH order would otherwise make bare `mpirun`
		// resolve to the 2.1.1 binary, whose pmix112 launcher talks to our
		// 4.1.6-linked ranks and fails with "PMIX UNPACK-PAST-END" /
		// "MPI_Init on a NULL communicator". (The image build now also purges
		// openmpi-bin, but the absolute path is the robust belt-and-suspenders.)
		caseComm := caseDir + "/study.comm"
		caseMesh := caseDir + "/mesh.med"
		run = fmt.Sprintf("for h in $(awk '{print $1}' /etc/mpi/hostfile 2>/dev/null); do n=0; until getent hosts \"$h\" >/dev/null 2>&1 || [ $n -ge 120 ]; do sleep 1; n=$((n+1)); done; done; /aster/openmpi/bin/mpirun --allow-run-as-root --mca routed radix --mca plm_rsh_no_tree_spawn 1 --mca oob_tcp_if_include eth0 --mca btl self,tcp --mca pml ob1 --mca btl_tcp_if_include eth0 -x PATH -x LD_LIBRARY_PATH -x PYTHONPATH -np %d bash -lc '. /aster/aster/share/aster/profile.sh; d=/tmp/ca/proc.$OMPI_COMM_WORLD_RANK; rm -rf $d && mkdir -p $d && cd $d; { echo \"from code_aster.Commands import *\"; echo \"from code_aster.Cata.Syntax import _F\"; echo \"from math import *\"; cat %s; } > fort.1; ln -sf %s fort.20; python3 fort.1 --memjeveux=2048 --tpmax=3600 --rep_outils=/aster/asrun/outils --rep_mat=/aster/aster/share/aster/materiau --rep_dex=/aster/aster/share/aster/datg --numthreads=1'", np, caseComm, caseMesh)
	default:
		return nil
	}

	script := fmt.Sprintf(
		"set -e; mkdir -p %s; cd %s; "+
			"export LD_PRELOAD=%s; export MPIP=\"-f %s\"; "+
			"%s; "+
			"cp -f *.mpiP %s/ 2>/dev/null || true",
		resultsDir, caseDir, mpiPLib, resultsDir, run, resultsDir,
	)
	return []string{"/bin/bash", "-lc", script}
}

// extractionCommand returns the bash command for the *pre-MPIJob* extraction
// Job (run as a regular Kubernetes Job, not as part of MPIJob). It does the
// solver-specific decomposition and emits an edge-list to
// /scheduler-graphs/<id>.edgelist that the topology-aware scheduler extender
// reads at placement time.
//
// The same solver image is used (no extra container to build) — the
// extraction binaries (extract-openfoam-graph, extract-radioss-graph) are
// baked into the runtime layer by docker/openfoam/Dockerfile and
// docker/openradioss/Dockerfile. Code_Aster uses a Python script
// (extract_codeaster_graph.py) bundled at /opt/scripts/.
//
// Output goes both into the case PVC (processor*/, parts/ — needed by the
// subsequent MPIJob) and into the shared scheduler-graphs PVC (edgelist —
// consumed by the extender).
func extractionCommand(t domain.SimulationType, simID, configPath string, np int) []string {
	caseDir := "/pvc/simulations/" + configPath
	edgelist := fmt.Sprintf("/scheduler-graphs/%s.edgelist", simID)

	switch t {
	case domain.SimTypeOpenFOAM:
		// The motorBike tutorial ships decomposeParDict.6 / -random but no
		// bare system/decomposeParDict, and the subdomain count must equal np
		// (a per-job parameter). Write a minimal scotch dict so one
		// decomposition feeds both the F-graph extraction here and the
		// simpleFoam -parallel run, which reads the processor* dirs off the
		// shared PVC.
		return []string{
			"/bin/bash", "-lc",
			fmt.Sprintf("set -e && cd %s && "+
				"{ echo 'FoamFile { version 2.0; format ascii; class dictionary; object decomposeParDict; }'; "+
				"echo 'numberOfSubdomains %d;'; echo 'method scotch;'; } > system/decomposeParDict && "+
				"decomposePar -force && extract-openfoam-graph . > %s", caseDir, np, edgelist),
		}
	case domain.SimTypeOpenRadioss:
		// The starter input deck differs by case origin: native OpenRadioss
		// cases ship a *_0000.rad Starter deck, while LS-Dyna imports (e.g. the
		// Yaris crash model) ship a .key master deck with units + includes baked
		// in. Resolve in priority order — native _0000.rad, then the
		// `main.{key,rad}` master convention, then any lone .rad — so both kinds
		// extract under one command. The starter emits input.graph0 regardless
		// of the deck name.
		return []string{
			"/bin/bash", "-lc",
			fmt.Sprintf("set -e && cd %s && "+
				"INPUT=\"$(ls *_0000.rad 2>/dev/null | head -1)\"; "+
				"[ -z \"$INPUT\" ] && INPUT=\"$(ls main.key main.rad 2>/dev/null | head -1)\"; "+
				"[ -z \"$INPUT\" ] && INPUT=\"$(ls *.rad 2>/dev/null | head -1)\"; "+
				"test -n \"$INPUT\" || { echo 'no OpenRadioss starter deck found' >&2; exit 1; }; "+
				"echo \"starter input: $INPUT\" && "+
				"starter_linux64_gf -np %d -input \"$INPUT\" && "+
				"gpmetis input.graph0 %d && "+
				"extract-radioss-graph input.graph0 input.graph0.part.%d > %s",
				caseDir, np, np, np, edgelist),
		}
	case domain.SimTypeCodeAster:
		// The image ships the low-level MED library + METIS CLI but NOT
		// MEDCoupling/medpartitioner, so the designed joints route can't run.
		// Build F(i,j) from shared nodes after an mpmetis element partition
		// (assets/extract_codeaster_graph_metis.py). The script is embedded in
		// the backend, not baked into the Code_Aster image, so it is written to
		// /tmp here. profile.sh puts the `med` Python binding on PYTHONPATH;
		// mpmetis lives in /aster/metis/bin.
		writeScript := "cat > /tmp/extract_ca.py <<'CAEOF'\n" + codeasterExtractScript + "\nCAEOF\n"
		runScript := fmt.Sprintf("python3 /tmp/extract_ca.py "+
			"--input \"$(ls *.med | head -1)\" --ndomains %d --output %s", np, edgelist)
		return []string{
			"/bin/bash", "-lc",
			"set -e && . /aster/aster/share/aster/profile.sh && " +
				"export PATH=\"$PATH:/aster/metis/bin\" && cd " + caseDir + " && " +
				writeScript + runScript,
		}
	}
	return nil
}

// SimulationManager creates MPIJob custom resources for engineering
// simulations. The topology-aware scheduler is selected via the
// `.spec.runPolicy.schedulerName` field of the MPIJob (which propagates
// to launcher and worker pods).
type SimulationManager struct {
	clientset *kubernetes.Clientset
	dynClient dynamic.Interface
	namespace string
}

func NewSimulationManager(clientset *kubernetes.Clientset, dynClient dynamic.Interface, namespace string) *SimulationManager {
	return &SimulationManager{
		clientset: clientset,
		dynClient: dynClient,
		namespace: namespace,
	}
}

// CreateExtractionJob creates a one-shot Kubernetes Job that runs the
// solver-specific extraction pipeline (decomposePar / Starter+gpmetis /
// medpartitioner+python) and writes the F-graph edgelist to the shared
// scheduler-graphs PVC. The caller (usecase) waits for this Job to reach
// Succeeded before creating the MPIJob — only then does the scheduler
// extender have an edgelist to read at MPIJob placement time.
//
// Naming: extract-<simID>. Garbage-collected by ownerReferences when the
// Simulation is deleted from the database.
func (m *SimulationManager) CreateExtractionJob(sim *domain.Simulation) error {
	image := solverImage(sim.Type)
	if image == "" {
		return fmt.Errorf("unsupported simulation type: %s", sim.Type)
	}
	np := sim.NumProcs
	if np < 1 {
		np = 1
	}
	cmd := extractionCommand(sim.Type, sim.ID, sim.ConfigPath, np)
	if cmd == nil {
		return fmt.Errorf("no extraction command for solver %s", sim.Type)
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("extract-%s", sim.ID),
			Namespace: m.namespace,
			Labels: map[string]string{
				"app":        "extraction",
				"type":       string(sim.Type),
				"mpi-job-id": sim.ID,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: int32Ptr(0), // fail fast on first error
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:  "extract",
						Image: image,
						// Use the kind-loaded image, never surprise-pull: a :latest tag
						// otherwise defaults to PullAlways and fetches the published image.
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command:         cmd,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "config", MountPath: "/pvc"},
							{Name: "scheduler-graphs", MountPath: "/scheduler-graphs"},
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "simulation-configs",
								},
							},
						},
						{
							Name: "scheduler-graphs",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "scheduler-graphs",
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := m.clientset.BatchV1().Jobs(m.namespace).Create(
		context.Background(), job, metav1.CreateOptions{},
	)
	return err
}

// GetExtractionStatus polls the extraction Job and returns one of:
//
//	pending  — Job created but not yet completed
//	succeeded — extraction wrote edgelist, MPIJob can be created
//	failed   — extraction errored; sim should be marked failed
func (m *SimulationManager) GetExtractionStatus(simID string) (string, error) {
	job, err := m.clientset.BatchV1().Jobs(m.namespace).Get(
		context.Background(), fmt.Sprintf("extract-%s", simID), metav1.GetOptions{},
	)
	if err != nil {
		return "", err
	}
	if job.Status.Succeeded > 0 {
		return "succeeded", nil
	}
	if job.Status.Failed > 0 {
		return "failed", nil
	}
	return "pending", nil
}

// DeleteExtractionJob removes the extraction Job and its pod (called after
// MPIJob is created and we no longer need the extraction artefact).
func (m *SimulationManager) DeleteExtractionJob(simID string) error {
	propagationPolicy := metav1.DeletePropagationBackground
	return m.clientset.BatchV1().Jobs(m.namespace).Delete(
		context.Background(),
		fmt.Sprintf("extract-%s", simID),
		metav1.DeleteOptions{PropagationPolicy: &propagationPolicy},
	)
}

func int32Ptr(i int32) *int32 { return &i }

func (m *SimulationManager) CreateJob(sim *domain.Simulation) error {
	image := solverImage(sim.Type)
	if image == "" {
		return fmt.Errorf("unsupported simulation type: %s", sim.Type)
	}
	np := sim.NumProcs
	if np < 1 {
		np = 1
	}

	mpiJob := &unstructured.Unstructured{}
	mpiJob.SetUnstructuredContent(map[string]any{
		"apiVersion": "kubeflow.org/v2beta1",
		"kind":       "MPIJob",
		"metadata": map[string]any{
			"name":      fmt.Sprintf("sim-%s", sim.ID),
			"namespace": m.namespace,
			"labels": map[string]any{
				"app":        "simulation",
				"type":       string(sim.Type),
				"mpi-job-id": sim.ID,
			},
		},
		"spec": map[string]any{
			"slotsPerWorker": int64(1),
			"runPolicy": map[string]any{
				"cleanPodPolicy": "Running",
				"schedulerName":  sim.SchedulerName,
			},
			"mpiReplicaSpecs": map[string]any{
				"Launcher": map[string]any{
					"replicas": int64(1),
					"template": map[string]any{
						"metadata": map[string]any{
							"labels": map[string]any{
								"mpi-job-id":   sim.ID,
								"mpi-role":     "launcher",
								LabelAlgorithm: sim.Algorithm,
							},
						},
						"spec": map[string]any{
							"schedulerName": sim.SchedulerName,
							"containers": []any{
								map[string]any{
									"name":            "solver",
									"image":           image,
									"imagePullPolicy": "IfNotPresent",
									"command":         toAnySlice(solverCommand(sim.Type, sim.ID, sim.ConfigPath, np)),
									"resources":       launcherResources(),
									"volumeMounts": []any{
										map[string]any{
											"name":      "config",
											"mountPath": "/pvc",
										},
										map[string]any{
											"name":      "results",
											"mountPath": "/results",
										},
									},
								},
							},
							"volumes": pvcVolumes(),
						},
					},
				},
				"Worker": map[string]any{
					"replicas": int64(np),
					"template": map[string]any{
						"metadata": map[string]any{
							"labels": map[string]any{
								"mpi-job-id":   sim.ID,
								"mpi-role":     "worker",
								LabelAlgorithm: sim.Algorithm,
							},
						},
						"spec": map[string]any{
							"schedulerName": sim.SchedulerName,
							"affinity":      workerAntiAffinity(sim.ID),
							"containers": []any{
								map[string]any{
									"name":            "solver",
									"image":           image,
									"imagePullPolicy": "IfNotPresent",
									"resources":       workerResources(),
									"volumeMounts": []any{
										map[string]any{
											"name":      "config",
											"mountPath": "/pvc",
										},
										map[string]any{
											"name":      "results",
											"mountPath": "/results",
										},
									},
								},
							},
							"volumes": pvcVolumes(),
						},
					},
				},
			},
		},
	})

	_, err := m.dynClient.Resource(MPIJobGVR).Namespace(m.namespace).Create(
		context.Background(), mpiJob, metav1.CreateOptions{},
	)
	return err
}

// launcherResources sets the resource request for the MPIJob launcher pod.
// The launcher only coordinates mpirun and does not do MPI work itself, so
// 100m CPU / 256Mi memory is enough. No CPU limit is set — see workerResources()
// for the rationale.
func launcherResources() map[string]any {
	return map[string]any{
		"requests": map[string]any{
			"cpu":    "100m",
			"memory": "256Mi",
		},
	}
}

// workerResources sets resources for each MPI rank's worker pod.
//
// Critical: NO CPU LIMITS — only requests. Setting limits=requests puts the
// pod into Guaranteed QoS, which triggers Linux CFS bandwidth throttling.
// Xie (arXiv:2603.22691, 2026) quantified the effect for tightly-coupled MPI:
// throttling on any single rank cascades through every MPI_Allreduce barrier
// and inflates wall-clock time by up to 78× (35s -> 2738s on pitzDaily).
//
// Requests-only puts the pod in Burstable QoS, where the kernel uses cpu.weight
// proportional sharing instead of hard quota: ranks can burst above their
// request when peers are idle at MPI barriers, eliminating the throttle.
func workerResources() map[string]any {
	return map[string]any{
		"requests": map[string]any{
			"cpu":    "1",
			"memory": "1Gi",
		},
	}
}

// workerAntiAffinity forces each MPI rank onto a separate node — one rank
// per vCPU per node, no oversubscription, no HT sharing. This matches the
// "clean experiment" topology used throughout the scheduling literature
// (Xie 2026, Beltre 2019, Queens IPDRM 2016) and gives the scheduler real
// placement choices over the available node pool.
func workerAntiAffinity(jobID string) map[string]any {
	return map[string]any{
		"podAntiAffinity": map[string]any{
			"requiredDuringSchedulingIgnoredDuringExecution": []any{
				map[string]any{
					"labelSelector": map[string]any{
						"matchExpressions": []any{
							map[string]any{
								"key":      "mpi-job-id",
								"operator": "In",
								"values":   []any{jobID},
							},
							map[string]any{
								"key":      "mpi-role",
								"operator": "In",
								"values":   []any{"worker"},
							},
						},
					},
					"topologyKey": "kubernetes.io/hostname",
				},
			},
		},
	}
}

func pvcVolumes() []any {
	return []any{
		map[string]any{
			"name": "config",
			"persistentVolumeClaim": map[string]any{
				"claimName": "simulation-configs",
			},
		},
		map[string]any{
			"name": "results",
			"persistentVolumeClaim": map[string]any{
				"claimName": "simulation-results",
			},
		},
	}
}

func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// GetJobStatus reads MPIJob.status.conditions and maps to our domain status.
func (m *SimulationManager) GetJobStatus(simID string) (domain.SimulationStatus, error) {
	obj, err := m.dynClient.Resource(MPIJobGVR).Namespace(m.namespace).Get(
		context.Background(), fmt.Sprintf("sim-%s", simID), metav1.GetOptions{},
	)
	if err != nil {
		return "", err
	}
	conds, ok, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !ok {
		return domain.SimStatusPending, nil
	}
	// MPIJob conditions follow Kubeflow common types: Created/Running/Succeeded/Failed.
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		ctype, _ := m["type"].(string)
		status, _ := m["status"].(string)
		if status != "True" {
			continue
		}
		switch ctype {
		case "Succeeded":
			return domain.SimStatusCompleted, nil
		case "Failed":
			return domain.SimStatusFailed, nil
		case "Running":
			return domain.SimStatusRunning, nil
		}
	}
	return domain.SimStatusPending, nil
}

func (m *SimulationManager) DeleteJob(simID string) error {
	propagationPolicy := metav1.DeletePropagationBackground
	err := m.dynClient.Resource(MPIJobGVR).Namespace(m.namespace).Delete(
		context.Background(),
		fmt.Sprintf("sim-%s", simID),
		metav1.DeleteOptions{PropagationPolicy: &propagationPolicy},
	)
	// An already-absent MPIJob is not an error: the mpi-operator may have
	// reaped it on BackoffLimitExceeded, or it was deleted out-of-band. The
	// caller still wants the simulation gone from the repository, so make the
	// delete idempotent instead of failing the whole request with a 500.
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
