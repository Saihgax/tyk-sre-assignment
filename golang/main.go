package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type server struct {
	clientset kubernetes.Interface
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
	app := &server{clientset: clientset}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/readyz", app.readyHandler)

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

func (s *server) readyHandler(w http.ResponseWriter, r *http.Request) {
	version, err := getKubernetesVersion(s.clientset)
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
