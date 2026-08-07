package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type app struct {
	clientset kubernetes.Interface
}

type deploymentHealth struct {
	Namespace         string `json:"namespace"`
	Name              string `json:"name"`
	DesiredReplicas   int32  `json:"desiredReplicas"`
	AvailableReplicas int32  `json:"availableReplicas"`
	Healthy           bool   `json:"healthy"`
}

type deploymentHealthResponse struct {
	Healthy     bool               `json:"healthy"`
	Deployments []deploymentHealth `json:"deployments"`
}

func main() {
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig, leave empty for in-cluster")
	listenAddr := flag.String("address", ":8080", "HTTP server listen address")

	flag.Parse()

	kConfig, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}

	clientset, err := kubernetes.NewForConfig(kConfig)
	if err != nil {
		panic(err)
	}

	version, err := getKubernetesVersion(clientset)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Connected to Kubernetes %s\n", version)

	if err := startServer(*listenAddr, clientset); err != nil {
		panic(err)
	}
}

// getKubernetesVersion returns a string GitVersion of the Kubernetes server defined by the clientset.
//
// If it can't connect an error will be returned, which makes it useful to check connectivity.
func getKubernetesVersion(clientset kubernetes.Interface) (string, error) {
	version, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return "", err
	}

	return version.String(), nil
}

// startServer launches an HTTP server with defined handlers and blocks until it's terminated or fails with an error.
//
// Expects a listenAddr to bind to.
func startServer(listenAddr string, clientset kubernetes.Interface) error {
	application := &app{clientset: clientset}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/readyz", application.readyHandler)                        // story 3
	mux.HandleFunc("/deployments/health", application.deploymentHealthHandler) // story 1

	fmt.Printf("Server listening on %s\n", listenAddr)

	return http.ListenAndServe(listenAddr, mux)
}

// healthHandler responds with the health status of the application.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)

	_, err := w.Write([]byte("ok"))
	if err != nil {
		fmt.Println("failed writing to response")
	}
}

// readyHandler reports whether the application can reach the Kubernetes API server.
func (a *app) readyHandler(w http.ResponseWriter, r *http.Request) {
	version, err := getKubernetesVersion(a.clientset)
	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "not ready",
			"error":  err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ready",
		"version": version,
	})
}

// getDeploymentHealth compares each Deployment's desired replicas with its available replicas.
func getDeploymentHealth(ctx context.Context, clientset kubernetes.Interface) (deploymentHealthResponse, error) {
	deployments, err := clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return deploymentHealthResponse{}, err
	}

	result := deploymentHealthResponse{
		Healthy:     true,
		Deployments: make([]deploymentHealth, 0, len(deployments.Items)),
	}

	for _, deployment := range deployments.Items {
		desired := int32(1)
		if deployment.Spec.Replicas != nil {
			desired = *deployment.Spec.Replicas
		}

		available := deployment.Status.AvailableReplicas
		healthy := desired == available

		result.Deployments = append(result.Deployments, deploymentHealth{
			Namespace:         deployment.Namespace,
			Name:              deployment.Name,
			DesiredReplicas:   desired,
			AvailableReplicas: available,
			Healthy:           healthy,
		})

		if !healthy {
			result.Healthy = false
		}
	}

	return result, nil
}

// deploymentHealthHandler returns the health of Deployments across the cluster.
func (a *app) deploymentHealthHandler(w http.ResponseWriter, r *http.Request) {
	result, err := getDeploymentHealth(r.Context(), a.clientset)
	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}
