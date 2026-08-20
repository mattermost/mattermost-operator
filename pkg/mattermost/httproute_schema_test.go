package mattermost

import (
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
)

// httpRouteCRDPath points at a pristine copy of the upstream Gateway API HTTPRoute
// CRD, standard channel.
//
// Source:
//
//	https://raw.githubusercontent.com/kubernetes-sigs/gateway-api/v1.6.1/config/crd/standard/gateway.networking.k8s.io_httproutes.yaml
//
// The Operator builds the HTTPRoute as unstructured rather than using the typed
// sigs.k8s.io/gateway-api SDK (see GenerateHTTPRouteV1Beta for why), so nothing in
// the Go compiler checks the field names or nesting we emit. These tests are that
// safety net, using the same schema the API server enforces.
//
// To target a newer Gateway API release, replace this file and re-run. A failure
// means the generated HTTPRoute is no longer valid against that release, which is
// the signal we want in CI rather than in a cluster.
//
// Only pruning is checked here, not full CRD validation. Pruning is what catches
// wrong field names, and it needs nothing beyond apiextensions-apiserver and
// apimachinery. Wiring up apiservervalidation.ValidateCustomResource as well would
// add k8s.io/apiserver, component-base, cel-go and antlr4 to the module graph for a
// test, which is not a trade worth making: the values we emit for constrained
// fields are compile-time constants in the generator, and numeric types are already
// covered by the JSON round trip assertion in TestGenerateHTTPRoute_V1Beta.
const httpRouteCRDPath = "testdata/gateway.networking.k8s.io_httproutes.yaml"

// loadHTTPRouteStructuralSchema returns the v1 HTTPRoute schema in the form the
// API server uses to decide which fields it recognises.
func loadHTTPRouteStructuralSchema(t *testing.T) *structuralschema.Structural {
	t.Helper()

	raw, err := os.ReadFile(httpRouteCRDPath)
	require.NoError(t, err, "vendored HTTPRoute CRD is missing")

	crd := &apiextensionsv1.CustomResourceDefinition{}
	require.NoError(t, yaml.Unmarshal(raw, crd))
	require.Equal(t, "httproutes.gateway.networking.k8s.io", crd.Name)

	var v1Schema *apiextensionsv1.JSONSchemaProps
	for i := range crd.Spec.Versions {
		if crd.Spec.Versions[i].Name == httpRouteGVK.Version {
			require.NotNil(t, crd.Spec.Versions[i].Schema, "CRD version has no schema")
			v1Schema = crd.Spec.Versions[i].Schema.OpenAPIV3Schema
			break
		}
	}
	require.NotNil(t, v1Schema, "CRD does not serve version %q", httpRouteGVK.Version)

	// Pruning operates on the internal apiextensions types.
	internal := &apiextensions.JSONSchemaProps{}
	require.NoError(t, apiextensionsv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(v1Schema, internal, nil))

	structural, err := structuralschema.NewStructural(internal)
	require.NoError(t, err)

	// Sanity check that a real schema loaded, so the assertions below cannot pass
	// vacuously against an empty one.
	require.Contains(t, structural.Properties, "spec")
	require.Contains(t, structural.Properties["spec"].Properties, "hostnames")

	return structural
}

// prunedPaths reports the fields of route that the schema does not recognise. These
// are exactly the fields a real API server would silently drop on write.
func prunedPaths(structural *structuralschema.Structural, route *unstructured.Unstructured) []string {
	opts := structuralschema.UnknownFieldPathOptions{TrackUnknownFieldPaths: true}
	return pruning.PruneWithOptions(route.DeepCopy().Object, structural, true, opts)
}

func schemaTestMattermost() *mmv1beta.Mattermost {
	return &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-mm",
			Namespace: "mm-ns",
		},
		Spec: mmv1beta.MattermostSpec{
			HTTPRoute: &mmv1beta.HTTPRouteSpec{
				Enabled: true,
				Host:    "mm.example.com",
				Hosts:   []mmv1beta.IngressHost{{HostName: "alt.example.com"}},
				GatewayRef: mmv1beta.GatewayReference{
					Name:        "shared-gateway",
					Namespace:   "gateway-system",
					SectionName: "https",
				},
				RequestTimeout:        "120s",
				BackendRequestTimeout: "60s",
			},
		},
	}
}

func TestGenerateHTTPRoute_ConformsToGatewayAPISchema(t *testing.T) {
	structural := loadHTTPRouteStructuralSchema(t)

	t.Run("fully configured route has no unknown fields", func(t *testing.T) {
		route := GenerateHTTPRouteV1Beta(schemaTestMattermost(), logr.Discard())

		assert.Empty(t, prunedPaths(structural, route),
			"these fields are not in the Gateway API schema and would be silently dropped")
	})

	t.Run("minimally configured route has no unknown fields", func(t *testing.T) {
		mattermost := &mmv1beta.Mattermost{
			ObjectMeta: metav1.ObjectMeta{Name: "my-mm", Namespace: "mm-ns"},
			Spec: mmv1beta.MattermostSpec{
				HTTPRoute: &mmv1beta.HTTPRouteSpec{
					Enabled:    true,
					Host:       "mm.example.com",
					GatewayRef: mmv1beta.GatewayReference{Name: "shared-gateway"},
				},
			},
		}

		route := GenerateHTTPRouteV1Beta(mattermost, logr.Discard())

		assert.Empty(t, prunedPaths(structural, route))
	})
}

// TestHTTPRouteSchemaCheckDetectsUnknownFields is a negative control. Without it, a
// schema that failed to load usefully would make the checks above pass vacuously.
func TestHTTPRouteSchemaCheckDetectsUnknownFields(t *testing.T) {
	structural := loadHTTPRouteStructuralSchema(t)

	specOf := func(route *unstructured.Unstructured) map[string]interface{} {
		return route.Object["spec"].(map[string]interface{})
	}

	t.Run("misspelled top level field", func(t *testing.T) {
		// The exact typo a typed SDK would have caught at compile time.
		route := GenerateHTTPRouteV1Beta(schemaTestMattermost(), logr.Discard())
		spec := specOf(route)
		spec["hostNames"] = spec["hostnames"]
		delete(spec, "hostnames")

		assert.Contains(t, prunedPaths(structural, route), "spec.hostNames")
	})

	t.Run("misspelled field inside a list element", func(t *testing.T) {
		route := GenerateHTTPRouteV1Beta(schemaTestMattermost(), logr.Discard())
		parentRef := specOf(route)["parentRefs"].([]interface{})[0].(map[string]interface{})
		parentRef["sectionname"] = parentRef["sectionName"]
		delete(parentRef, "sectionName")

		pruned := prunedPaths(structural, route)
		require.NotEmpty(t, pruned)
		assert.Contains(t, pruned[0], "sectionname")
	})

	t.Run("field that does not exist at that nesting level", func(t *testing.T) {
		// timeouts belongs on the rule, not on the backendRef.
		route := GenerateHTTPRouteV1Beta(schemaTestMattermost(), logr.Discard())
		rule := specOf(route)["rules"].([]interface{})[0].(map[string]interface{})
		backendRef := rule["backendRefs"].([]interface{})[0].(map[string]interface{})
		backendRef["timeouts"] = rule["timeouts"]

		pruned := prunedPaths(structural, route)
		require.NotEmpty(t, pruned)
		assert.Contains(t, pruned[0], "timeouts")
	})
}
