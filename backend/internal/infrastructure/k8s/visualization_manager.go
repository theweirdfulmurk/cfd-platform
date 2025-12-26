package k8s

import (
	"context"
	"fmt"

	"github.com/theweirdfulmurk/cfd-platform/internal/domain"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type VisualizationManager struct {
	clientset *kubernetes.Clientset
	namespace string
}

func NewVisualizationManager(clientset *kubernetes.Clientset, namespace string) *VisualizationManager {
	return &VisualizationManager{
		clientset: clientset,
		namespace: namespace,
	}
}

func (m *VisualizationManager) CreatePod(vizID, resultPath string) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("viz-%s", vizID),
			Labels: map[string]string{
				"app":  "paraview-viz",
				"type": "visualization",
				"id":   vizID,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "paraview",
					Image: "kitware/paraview:pvw-v5.7.1-osmesa-py2",
					Command: []string{
						"/bin/bash", "-c",
						"cd /pvw && python -m light_viz.server --port 9000 --data /data",
					},
					Ports: []corev1.ContainerPort{
						{
							Name:          "ws",
							ContainerPort: 9000,
							Protocol:      corev1.ProtocolTCP,
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "results",
							MountPath: "/data",
							SubPath:   resultPath,
						},
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("2Gi"),
							corev1.ResourceCPU:    resource.MustParse("1"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("4Gi"),
							corev1.ResourceCPU:    resource.MustParse("2"),
						},
					},
				},
			},
			Volumes: []corev1.Volume{
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
	}

	_, err := m.clientset.CoreV1().Pods(m.namespace).Create(
		context.Background(),
		pod,
		metav1.CreateOptions{},
	)
	return err
}

func (m *VisualizationManager) GetPodStatus(vizID string) (domain.VisualizationStatus, error) {
	pod, err := m.clientset.CoreV1().Pods(m.namespace).Get(
		context.Background(),
		fmt.Sprintf("viz-%s", vizID),
		metav1.GetOptions{},
	)
	if err != nil {
		return "", err
	}

	switch pod.Status.Phase {
	case corev1.PodRunning:
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.ContainersReady && cond.Status == corev1.ConditionTrue {
				return domain.VizStatusReady, nil
			}
		}
		return domain.VizStatusRunning, nil
	case corev1.PodPending:
		return domain.VizStatusPending, nil
	case corev1.PodFailed:
		return domain.VizStatusFailed, nil
	default:
		return domain.VizStatusPending, nil
	}
}

func (m *VisualizationManager) GetPodIP(vizID string) (string, error) {
	pod, err := m.clientset.CoreV1().Pods(m.namespace).Get(
		context.Background(),
		fmt.Sprintf("viz-%s", vizID),
		metav1.GetOptions{},
	)
	if err != nil {
		return "", err
	}

	if pod.Status.PodIP == "" {
		return "", fmt.Errorf("pod IP not assigned yet")
	}

	return pod.Status.PodIP, nil
}

func (m *VisualizationManager) DeletePod(vizID string) error {
	return m.clientset.CoreV1().Pods(m.namespace).Delete(
		context.Background(),
		fmt.Sprintf("viz-%s", vizID),
		metav1.DeleteOptions{},
	)
}