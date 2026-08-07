package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	disco "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetKubernetesVersion(t *testing.T) {
	okClientset := fake.NewSimpleClientset()
	okClientset.Discovery().(*disco.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: "1.25.0-fake"}

	okVer, err := getKubernetesVersion(okClientset)
	assert.NoError(t, err)
	assert.Equal(t, "1.25.0-fake", okVer)

	badClientset := fake.NewSimpleClientset()
	badClientset.Discovery().(*disco.FakeDiscovery).FakedServerVersion = &version.Info{}

	badVer, err := getKubernetesVersion(badClientset)
	assert.NoError(t, err)
	assert.Equal(t, "", badVer)
}

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	healthHandler(rec, req)
	res := rec.Result()

	assert.Equal(t, http.StatusOK, res.StatusCode)

	defer func(Body io.ReadCloser) {
		assert.NoError(t, Body.Close())
	}(res.Body)
	resp, err := io.ReadAll(res.Body)

	assert.NoError(t, err)
	assert.Equal(t, "ok", string(resp))
}

func TestReadyHandler(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clientset.Discovery().(*disco.FakeDiscovery).FakedServerVersion = &version.Info{
		GitVersion: "1.25.0-fake",
	}

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	application := &app{clientset: clientset}
	application.readyHandler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t,
		`{"status":"ready","version":"1.25.0-fake"}`,
		rec.Body.String(),
	)
}

func TestGetDeploymentHealth(t *testing.T) {
	apiDesired := int32(2)
	workerDesired := int32(3)

	clientset := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api",
				Namespace: "default",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &apiDesired,
			},
			Status: appsv1.DeploymentStatus{
				AvailableReplicas: 2,
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "worker",
				Namespace: "jobs",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &workerDesired,
			},
			Status: appsv1.DeploymentStatus{
				AvailableReplicas: 1,
			},
		},
	)

	result, err := getDeploymentHealth(context.Background(), clientset)

	assert.NoError(t, err)
	assert.False(t, result.Healthy)
	assert.Len(t, result.Deployments, 2)
}
