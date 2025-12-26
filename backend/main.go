package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
)

const (
	JobTypeCFD = "cfd"
	JobTypeFEA = "fea"
)

type Job struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	InputFile   string     `json:"input_file"`
	ResultsPath string     `json:"results_path,omitempty"`
	Error       string     `json:"error,omitempty"`
}

type JobStore struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

func NewJobStore() *JobStore {
	return &JobStore{
		jobs: make(map[string]*Job),
	}
}

func (s *JobStore) Create(job *Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
}

func (s *JobStore) Get(id string) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	return job, ok
}

func (s *JobStore) GetAll() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

func (s *JobStore) Update(id string, updateFn func(*Job)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		updateFn(job)
	}
}

type Server struct {
	k8sClient *kubernetes.Clientset
	jobStore  *JobStore
	dataDir   string
	namespace string
}

func NewServer(k8sClient *kubernetes.Clientset, dataDir, namespace string) *Server {
	return &Server{
		k8sClient: k8sClient,
		jobStore:  NewJobStore(),
		dataDir:   dataDir,
		namespace: namespace,
	}
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form
	err := r.ParseMultipartForm(32 << 20) // 32 MB max
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	jobType := r.FormValue("type")
	if jobType != JobTypeCFD && jobType != JobTypeFEA {
		http.Error(w, "Invalid job type. Must be 'cfd' or 'fea'", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("input")
	if err != nil {
		http.Error(w, "Failed to get input file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Create job
	job := &Job{
		ID:        uuid.New().String(),
		Type:      jobType,
		Status:    "submitted",
		CreatedAt: time.Now(),
	}

	// Save input file
	inputDir := filepath.Join(s.dataDir, "inputs", job.ID)
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		http.Error(w, "Failed to create input directory", http.StatusInternalServerError)
		return
	}

	// Determine filename based on job type
	var inputFilename string
	if jobType == JobTypeFEA {
		inputFilename = "input.inp"
	} else {
		inputFilename = header.Filename
	}

	inputPath := filepath.Join(inputDir, inputFilename)
	dst, err := os.Create(inputPath)
	if err != nil {
		http.Error(w, "Failed to save input file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		http.Error(w, "Failed to save input file", http.StatusInternalServerError)
		return
	}

	job.InputFile = header.Filename
	s.jobStore.Create(job)

	// Create Kubernetes job
	go s.runK8sJob(job)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing job id", http.StatusBadRequest)
		return
	}

	job, ok := s.jobStore.Get(id)
	if !ok {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobs := s.jobStore.GetAll()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

func (s *Server) handleGetResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing job id", http.StatusBadRequest)
		return
	}

	job, ok := s.jobStore.Get(id)
	if !ok {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	if job.Status != "completed" {
		http.Error(w, "Job not completed yet", http.StatusBadRequest)
		return
	}

	resultsDir := filepath.Join(s.dataDir, "inputs", id)

	// Создаем tar.gz архив
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=results-%s.tar.gz", id[:8]))

	// Архивируем результаты
	gzWriter := gzip.NewWriter(w)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	filepath.Walk(resultsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		header, _ := tar.FileInfoHeader(info, "")
		header.Name = filepath.Base(path)
		tarWriter.WriteHeader(header)

		file, _ := os.Open(path)
		io.Copy(tarWriter, file)
		file.Close()
		return nil
	})
}

func (s *Server) runK8sJob(job *Job) {
	ctx := context.Background()

	// Update job status to running
	s.jobStore.Update(job.ID, func(j *Job) {
		j.Status = "running"
	})

	// Determine solver image based on job type
	var image string
	var command []string

	switch job.Type {
	case JobTypeCFD:
		image = "openfoam/openfoam10-paraview56"
		command = []string{"/bin/bash", "-c", "source /opt/openfoam10/etc/bashrc && cd /data && tar --strip-components=1 -xzf *.tar.gz && icoFoam && tar -czf results.tar.gz [0-9]* constant/polyMesh"}
	case JobTypeFEA:
		image = "unifem/openfoam-ccx"
		command = []string{"/bin/bash", "-c", "cd /data && ccx -i input"}
	}

	// Create Kubernetes Job
	jobName := fmt.Sprintf("solver-%s", job.ID[:8])
	k8sJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: s.namespace,
			Labels: map[string]string{
				"app":      "cfd-platform",
				"job-id":   job.ID,
				"job-type": job.Type,
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
									Name:      "data",
									MountPath: "/data",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: filepath.Join(s.dataDir, "inputs", job.ID),
									Type: ptr.To(corev1.HostPathDirectory),
								},
							},
						},
					},
				},
			},
			BackoffLimit: ptr.To(int32(2)),
		},
	}

	// Create the job
	_, err := s.k8sClient.BatchV1().Jobs(s.namespace).Create(ctx, k8sJob, metav1.CreateOptions{})
	if err != nil {
		log.Printf("Failed to create Kubernetes job: %v", err)
		s.jobStore.Update(job.ID, func(j *Job) {
			j.Status = "failed"
			j.Error = err.Error()
			now := time.Now()
			j.CompletedAt = &now
		})
		return
	}

	// Monitor job status
	s.monitorK8sJob(jobName, job.ID)
}

func (s *Server) monitorK8sJob(jobName, jobID string) {
	ctx := context.Background()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	timeout := time.After(30 * time.Minute)

	for {
		select {
		case <-ticker.C:
			k8sJob, err := s.k8sClient.BatchV1().Jobs(s.namespace).Get(ctx, jobName, metav1.GetOptions{})
			if err != nil {
				log.Printf("Failed to get job status: %v", err)
				continue
			}

			if k8sJob.Status.Succeeded > 0 {
				s.jobStore.Update(jobID, func(j *Job) {
					j.Status = "completed"
					now := time.Now()
					j.CompletedAt = &now
				})
				return
			}

			if k8sJob.Status.Failed > 0 {
				s.jobStore.Update(jobID, func(j *Job) {
					j.Status = "failed"
					j.Error = "Job failed in Kubernetes"
					now := time.Now()
					j.CompletedAt = &now
				})
				return
			}

		case <-timeout:
			s.jobStore.Update(jobID, func(j *Job) {
				j.Status = "failed"
				j.Error = "Job timeout"
				now := time.Now()
				j.CompletedAt = &now
			})
			return
		}
	}
}

func main() {
	// Get Kubernetes config
	var config *rest.Config
	var err error

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}

	config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		// Try in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			log.Fatalf("Failed to get Kubernetes config: %v", err)
		}
	}

	// Create Kubernetes client
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	// Create data directory
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/tmp/cfd-platform"
	}
	os.MkdirAll(filepath.Join(dataDir, "inputs"), 0755)
	os.MkdirAll(filepath.Join(dataDir, "results"), 0755)

	namespace := os.Getenv("K8S_NAMESPACE")
	if namespace == "" {
		namespace = "cfd-platform"
	}

	server := NewServer(clientset, dataDir, namespace)

	// Setup HTTP handlers
	http.HandleFunc("/api/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		// Enable CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			return
		}

		if r.Method == http.MethodPost {
			server.handleCreateJob(w, r)
		} else {
			server.handleListJobs(w, r)
		}
	})

	http.HandleFunc("/api/v1/jobs/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			return
		}

		server.handleGetJob(w, r)
	})

	http.HandleFunc("/api/v1/results", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		server.handleGetResults(w, r)
	})

	// Serve frontend
	fs := http.FileServer(http.Dir("../frontend"))
	http.Handle("/", fs)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
