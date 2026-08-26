package v1beta1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// validSpec returns a spec that passes SetDefaults, so these tests can focus on
// sizing rather than on the database and file store requirements.
func validSpec() MattermostSpec {
	return MattermostSpec{
		Ingress:   &Ingress{Enabled: false},
		FileStore: FileStore{External: &ExternalFileStore{URL: "s3.example.com"}},
		Database:  Database{External: &ExternalDatabase{Secret: "db-secret"}},
	}
}

// TestSizeDefaultsAreNotShared guards against the size presets being aliased
// rather than copied. The presets are package-level values, so handing out a
// pointer into one - or assigning a ResourceRequirements, whose ResourceLists are
// maps - lets one Mattermost mutate the preset and change the defaults every
// Mattermost reconciled afterwards receives.
func TestSizeDefaultsAreNotShared(t *testing.T) {
	originalReplicas := DefaultSize.App.Replicas
	originalCPU := DefaultSize.App.Resources.Requests.Cpu().String()

	first := &Mattermost{Spec: validSpec()}
	require.NoError(t, first.SetReplicasAndResourcesFromSize())

	// Mutate the first resource the way a controller or a user might.
	*first.Spec.Replicas = originalReplicas + 997
	first.Spec.Scheduling.Resources.Requests[corev1.ResourceCPU] = resource.MustParse("777m")

	assert.Equal(t, originalReplicas, DefaultSize.App.Replicas,
		"mutating a Mattermost must not change the shared size preset")
	assert.Equal(t, originalCPU, DefaultSize.App.Resources.Requests.Cpu().String(),
		"mutating a Mattermost must not change the shared size preset's resources")

	second := &Mattermost{Spec: validSpec()}
	require.NoError(t, second.SetReplicasAndResourcesFromSize())

	assert.Equal(t, originalReplicas, *second.Spec.Replicas,
		"a later Mattermost must not inherit another one's mutations")
	assert.Equal(t, originalCPU, second.Spec.Scheduling.Resources.Requests.Cpu().String())
}

// TestExplicitSizeIsNotShared is the same guarantee for the spec.size path.
//
// The resource maps are the part that can actually leak here: GetMattermostSize
// returns a Size by value and overrideReplicasAndResourcesFromSize takes one by
// value, but copying the struct copies the ResourceList map headers, so the maps
// are still shared with the preset without a DeepCopy.
//
// The replica assertion cannot fail for that same reason - the pointer would
// point into the per-call copy rather than the preset - and is kept only to state
// the expectation. Replica aliasing is caught by TestSizeDefaultsAreNotShared,
// where setDefaultReplicasAndResources reads the package-level DefaultSize
// directly and a pointer into it really is shared process-wide.
func TestExplicitSizeIsNotShared(t *testing.T) {
	preset, err := GetMattermostSize(Size1000String)
	require.NoError(t, err)
	originalReplicas := preset.App.Replicas
	originalCPU := preset.App.Resources.Requests.Cpu().String()

	spec := validSpec()
	spec.Size = Size1000String
	mm := &Mattermost{Spec: spec}
	require.NoError(t, mm.SetReplicasAndResourcesFromSize())

	*mm.Spec.Replicas = originalReplicas + 1
	mm.Spec.Scheduling.Resources.Requests[corev1.ResourceCPU] = resource.MustParse("888m")

	after, err := GetMattermostSize(Size1000String)
	require.NoError(t, err)
	assert.Equal(t, originalReplicas, after.App.Replicas,
		"mutating a Mattermost's replicas must not change the named size preset")
	assert.Equal(t, originalCPU, after.App.Resources.Requests.Cpu().String(),
		"mutating a Mattermost must not change the named size preset")
}
