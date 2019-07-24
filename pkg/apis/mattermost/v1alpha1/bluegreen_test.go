package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestBlueGreen(t *testing.T) {
	ci := &BlueGreen{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: BlueGreenSpec{
			InstallationName:    "mm-foo",
			Version:     "5.11.0",
			IngressName: "foo.mattermost.dev",
		},
	}

	t.Run("scheme", func(t *testing.T) {
		err := SchemeBuilder.AddToScheme(scheme.Scheme)
		require.NoError(t, err)
	})

	t.Run("deepcopy", func(t *testing.T) {
		t.Run("blue green", func(t *testing.T) {
			require.Equal(t, ci, ci.DeepCopy())
		})
		t.Run("blue green list", func(t *testing.T) {
			cil := &BlueGreenList{
				Items: []BlueGreen{
					*ci,
				},
			}
			require.Equal(t, cil, cil.DeepCopy())
		})
	})
}
