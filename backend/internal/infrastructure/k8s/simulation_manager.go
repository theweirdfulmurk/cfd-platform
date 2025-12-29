package k8s

import (
	"context"
	"fmt"

	"github.com/theweirdfulmurk/cfd-platform/internal/domain"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type SimulationManager struct {
	clientset *kubernetes.Clientset
	namespace string
}

func NewSimulationManager(clientset *kubernetes.Clientset, namespace string) *SimulationManager {
	return &SimulationManager{
		clientset: clientset,
		namespace: namespace,
	}
}

func (m *SimulationManager) CreateJob(simID string, simType domain.SimulationType, configPath string) error {
	var image string
	var command []string

	switch simType {
	case domain.SimTypeCFD:
		image = "openfoam/openfoam8-paraview56"
		command = []string{"/bin/bash", "-c",
			"cd /pvc/simulations/" + configPath + " && tar -xzf *.tar.gz && ./Allrun"}
	case domain.SimTypeFEA:
		image = "calculix/ccx:latest"
		command = []string{"/bin/bash", "-c", 
			"mkdir -p /results/" + configPath + " && cp /pvc/simulations/" + configPath + "/input.inp /tmp/ && cd /tmp && ccx input && cp *.frd *.dat /results/" + configPath + "/"}
	default:
		return fmt.Errorf("unsupported simulation type: %s", simType)
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("sim-%s", simID),
			Labels: map[string]string{
				"app":  "simulation",
				"type": string(simType),
				"id":   simID,
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						FSGroup: int64Ptr(1000),
					},
					Containers: []corev1.Container{
						{
							Name:       "solver",
							Image:      image,
							Command:    command,
							WorkingDir: "/pvc/simulations/" + configPath,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/pvc",
									ReadOnly:  false,
								},
								{
									Name:      "results",
									MountPath: "/results",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("512Mi"), // было 4Gi
									corev1.ResourceCPU:    resource.MustParse("500m"),  // было 2
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"), // было 8Gi
									corev1.ResourceCPU:    resource.MustParse("1"),   // было 4
								},
							},
						},
					},
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
							Name: "results",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "simulation-results",
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := m.clientset.BatchV1().Jobs(m.namespace).Create(
		context.Background(),
		job,
		metav1.CreateOptions{},
	)
	return err
}

func (m *SimulationManager) GetJobStatus(simID string) (domain.SimulationStatus, error) {
    job, err := m.clientset.BatchV1().Jobs(m.namespace).Get(
        context.Background(),
        fmt.Sprintf("sim-%s", simID),
        metav1.GetOptions{},
    )
    if err != nil {
        return "", err
    }

    // Проверь условия
    for _, condition := range job.Status.Conditions {
        if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
            return domain.SimStatusCompleted, nil
        }
        if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
            return domain.SimStatusFailed, nil
        }
    }

    if job.Status.Active > 0 {
        return domain.SimStatusRunning, nil
    }

    return domain.SimStatusPending, nil
}

func (m *SimulationManager) DeleteJob(simID string) error {
	propagationPolicy := metav1.DeletePropagationBackground
	return m.clientset.BatchV1().Jobs(m.namespace).Delete(
		context.Background(),
		fmt.Sprintf("sim-%s", simID),
		metav1.DeleteOptions{
			PropagationPolicy: &propagationPolicy,
		},
	)
}

func int64Ptr(i int64) *int64 {
	return &i
}
