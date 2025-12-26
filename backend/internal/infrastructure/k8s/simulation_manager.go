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
		command = []string{"/bin/bash", "-c", "cd /data && ./Allrun"}
	case domain.SimTypeFEA:
		image = "calculix/ccx:latest"
		command = []string{"/bin/bash", "-c", "ccx -i /data/input"}
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
					Containers: []corev1.Container{
						{
							Name:    "solver",
							Image:   image,
							Command: command,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/data",
									SubPath:   configPath,
								},
								{
									Name:      "results",
									MountPath: "/results",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("4Gi"),
									corev1.ResourceCPU:    resource.MustParse("2"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("8Gi"),
									corev1.ResourceCPU:    resource.MustParse("4"),
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

	if job.Status.Succeeded > 0 {
		return domain.SimStatusCompleted, nil
	}
	if job.Status.Failed > 0 {
		return domain.SimStatusFailed, nil
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