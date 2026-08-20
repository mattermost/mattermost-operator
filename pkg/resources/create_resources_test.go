package resources

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// getErrorClient returns a fixed error from Get, delegating everything else to an
// embedded client. The fake client cannot reproduce these cases: its Get resolves
// through the scheme and tracker rather than a RESTMapper, so it never produces the
// no-kind-match error a real client returns for an uninstalled CRD.
type getErrorClient struct {
	client.Client
	err error
}

func (c getErrorClient) Get(_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return c.err
}

func noGatewayAPIError() error {
	return &meta.NoKindMatchError{
		GroupKind:        schema.GroupKind{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute"},
		SearchedVersions: []string{"v1"},
	}
}

func helperWithGetError(err error) *ResourceHelper {
	base := fake.NewClientBuilder().Build()
	return NewResourceHelper(getErrorClient{Client: base, err: err}, runtime.NewScheme())
}

func TestDeleteHTTPRoute(t *testing.T) {
	key := types.NamespacedName{Name: "mm", Namespace: "default"}

	t.Run("absent HTTPRoute is not an error", func(t *testing.T) {
		helper := helperWithGetError(k8sErrors.NewNotFound(
			schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "httproutes"}, "mm"))

		assert.NoError(t, helper.DeleteHTTPRoute(key, logr.Discard()))
	})

	t.Run("uninstalled Gateway API is not an error", func(t *testing.T) {
		// Regression test. Clusters without the Gateway API CRDs are the common case,
		// and this delete runs on every reconcile of every installation that does not
		// use HTTPRoute. Returning an error here aborted the reconcile before the
		// Deployment was reconciled, breaking installations that never opted in.
		helper := helperWithGetError(noGatewayAPIError())

		assert.NoError(t, helper.DeleteHTTPRoute(key, logr.Discard()),
			"a missing Gateway API must be treated as nothing to delete")
	})

	t.Run("other errors still propagate", func(t *testing.T) {
		helper := helperWithGetError(k8sErrors.NewForbidden(
			schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "httproutes"},
			"mm", assert.AnError))

		err := helper.DeleteHTTPRoute(key, logr.Discard())
		require.Error(t, err, "a permissions problem must not be silently swallowed")
		assert.Contains(t, err.Error(), "failed to check if HTTPRoute exists")
	})
}

func TestCreateHTTPRouteIfNotExists_MissingGatewayAPIFailsLoudly(t *testing.T) {
	// The mirror of the case above: if an installation explicitly asks for an
	// HTTPRoute on a cluster that cannot serve one, that must surface rather than
	// being silently skipped.
	helper := helperWithGetError(noGatewayAPIError())

	owner := &metav1.ObjectMeta{Name: "mm", Namespace: "default"}
	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute",
	})
	route.SetName("mm")
	route.SetNamespace("default")

	err := helper.CreateHTTPRouteIfNotExists(owner, route, logr.Discard())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check if HTTPRoute exists")
}
