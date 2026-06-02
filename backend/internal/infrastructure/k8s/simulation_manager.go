package k8s

import (
	"context"
	"fmt"

	"github.com/theweirdfulmurk/cfd-platform/internal/domain"
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

// solverCommand returns the launcher command (inside the MPIJob launcher
// pod) that kicks off the solver across all worker pods via mpirun. Each
// solver has its own entry-point.
func solverCommand(t domain.SimulationType, configPath string, np int) []string {
	caseDir := "/pvc/simulations/" + configPath
	switch t {
	case domain.SimTypeOpenFOAM:
		return []string{
			"/bin/bash", "-c",
			fmt.Sprintf("cd %s && decomposePar -force && "+
				"mpirun -np %d simpleFoam -parallel", caseDir, np),
		}
	case domain.SimTypeOpenRadioss:
		return []string{
			"/bin/bash", "-c",
			fmt.Sprintf("cd %s && starter_linux64_gf -np %d -input *.rad && "+
				"mpirun -np %d engine_linux64_gf_ompi", caseDir, np, np),
		}
	case domain.SimTypeCodeAster:
		return []string{
			"/bin/bash", "-c",
			fmt.Sprintf("cd %s && mpirun -np %d as_run *.export", caseDir, np),
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
				"app":    "simulation",
				"type":   string(sim.Type),
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
								"mpi-job-id": sim.ID,
								"mpi-role":   "launcher",
							},
						},
						"spec": map[string]any{
							"schedulerName": sim.SchedulerName,
							"containers": []any{
								map[string]any{
									"name":    "solver",
									"image":   image,
									"command": toAnySlice(solverCommand(sim.Type, sim.ConfigPath, np)),
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
								"mpi-job-id": sim.ID,
								"mpi-role":   "worker",
							},
						},
						"spec": map[string]any{
							"schedulerName": sim.SchedulerName,
							"containers": []any{
								map[string]any{
									"name":  "solver",
									"image": image,
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
	return m.dynClient.Resource(MPIJobGVR).Namespace(m.namespace).Delete(
		context.Background(),
		fmt.Sprintf("sim-%s", simID),
		metav1.DeleteOptions{PropagationPolicy: &propagationPolicy},
	)
}
