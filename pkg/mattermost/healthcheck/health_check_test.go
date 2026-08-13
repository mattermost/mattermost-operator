package healthcheck

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func httpRouteScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	s.AddKnownTypeWithName(httpRouteListGVK.GroupVersion().WithKind("HTTPRoute"), &unstructured.Unstructured{})
	s.AddKnownTypeWithName(httpRouteListGVK, &unstructured.UnstructuredList{})
	return s
}

func newHTTPRoute(name, namespace string, labels map[string]string, hostnames ...string) *unstructured.Unstructured {
	hosts := make([]interface{}, 0, len(hostnames))
	for _, h := range hostnames {
		hosts = append(hosts, h)
	}

	route := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "HTTPRoute",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"hostnames": hosts,
		},
	}}
	route.SetLabels(labels)

	return route
}

func TestCheckHTTPRoute(t *testing.T) {
	mmLabels := map[string]string{
		"app": "mattermost",
		"installation.mattermost.com/installation": "foo",
		"installation.mattermost.com/resource":     "foo",
	}

	listOptions := []client.ListOption{
		client.InNamespace("default"),
		client.MatchingLabels(mmLabels),
	}

	newChecker := func(t *testing.T, objs ...client.Object) *HealthChecker {
		t.Helper()
		c := fake.NewClientBuilder().
			WithScheme(httpRouteScheme(t)).
			WithObjects(objs...).
			Build()
		return NewHealthChecker(c, listOptions, logr.Discard())
	}

	t.Run("returns the primary hostname", func(t *testing.T) {
		checker := newChecker(t,
			newHTTPRoute("foo", "default", mmLabels, "mm.example.com", "extra.example.com"))

		endpoint, err := checker.CheckHTTPRoute()
		require.NoError(t, err)
		assert.Equal(t, "mm.example.com", endpoint)
	})

	t.Run("errors when no HTTPRoute exists", func(t *testing.T) {
		checker := newChecker(t)

		_, err := checker.CheckHTTPRoute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected one HTTPRoute, but found 0")
	})

	t.Run("errors when more than one HTTPRoute matches", func(t *testing.T) {
		checker := newChecker(t,
			newHTTPRoute("foo", "default", mmLabels, "mm.example.com"),
			newHTTPRoute("bar", "default", mmLabels, "other.example.com"))

		_, err := checker.CheckHTTPRoute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected one HTTPRoute, but found 2")
	})

	t.Run("errors when the HTTPRoute has no hostnames", func(t *testing.T) {
		checker := newChecker(t, newHTTPRoute("foo", "default", mmLabels))

		_, err := checker.CheckHTTPRoute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no hostnames")
	})

	t.Run("ignores routes in other namespaces", func(t *testing.T) {
		checker := newChecker(t,
			newHTTPRoute("foo", "default", mmLabels, "mm.example.com"),
			newHTTPRoute("foo", "other-ns", mmLabels, "other.example.com"))

		endpoint, err := checker.CheckHTTPRoute()
		require.NoError(t, err)
		assert.Equal(t, "mm.example.com", endpoint)
	})

	t.Run("ignores routes belonging to another installation", func(t *testing.T) {
		otherLabels := map[string]string{
			"app": "mattermost",
			"installation.mattermost.com/installation": "bar",
			"installation.mattermost.com/resource":     "bar",
		}
		checker := newChecker(t,
			newHTTPRoute("foo", "default", mmLabels, "mm.example.com"),
			newHTTPRoute("bar", "default", otherLabels, "bar.example.com"))

		endpoint, err := checker.CheckHTTPRoute()
		require.NoError(t, err)
		assert.Equal(t, "mm.example.com", endpoint)
	})
}
