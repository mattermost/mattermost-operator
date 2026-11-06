package resources

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mmv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
)

func newOwner(name, namespace, uid string) *mmv1beta.Mattermost {
	return &mmv1beta.Mattermost{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: types.UID(uid)}}
}

func controllerRef(apiVersion, kind, name, uid string) []metav1.OwnerReference {
	isController := true
	return []metav1.OwnerReference{{APIVersion: apiVersion, Kind: kind, Name: name, UID: types.UID(uid), Controller: &isController}}
}

func TestResourceHelper_Update_Ownership(t *testing.T) {
	owner := newOwner("mm", "tenant", "owner-uid")
	logger := logr.Discard()

	newService := func(namespace string, refs []metav1.OwnerReference) *corev1.Service {
		return &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: namespace, OwnerReferences: refs},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}},
		}
	}

	for _, testCase := range []struct {
		description string
		current     *corev1.Service
		allowed     bool
	}{
		{"controlled by owner", newService("tenant", controllerRef("installation.mattermost.com/v1beta1", "Mattermost", "mm", "owner-uid")), true},
		{"no controller in owner namespace", newService("tenant", nil), true},
		{"controlled by same-named ClusterInstallation", newService("tenant", controllerRef(mmv1alpha1.GroupVersion.String(), "ClusterInstallation", "mm", "ci-uid")), true},
		{"controlled by other ClusterInstallation", newService("tenant", controllerRef(mmv1alpha1.GroupVersion.String(), "ClusterInstallation", "other", "ci-uid")), false},
		{"controlled by other Mattermost", newService("tenant", controllerRef("installation.mattermost.com/v1beta1", "Mattermost", "other", "other-uid")), false},
		{"same name in foreign group", newService("tenant", controllerRef("evil.example.com/v1", "ClusterInstallation", "mm", "x")), false},
		{"no controller in other namespace", newService("kube-system", nil), false},
		{"controlled by ClusterInstallation in other namespace", newService("kube-system", controllerRef(mmv1alpha1.GroupVersion.String(), "ClusterInstallation", "mm", "ci-uid")), false},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			c := fake.NewClientBuilder().WithObjects(testCase.current.DeepCopy()).Build()
			helper := NewResourceHelper(c, scheme.Scheme)

			current := &corev1.Service{}
			require.NoError(t, c.Get(context.TODO(), types.NamespacedName{Name: "svc", Namespace: testCase.current.Namespace}, current))

			desired := newService(testCase.current.Namespace, nil)
			desired.Spec.Ports = []corev1.ServicePort{{Port: 8065}}

			err := helper.Update(owner, current, desired, logger)

			after := &corev1.Service{}
			require.NoError(t, c.Get(context.TODO(), types.NamespacedName{Name: "svc", Namespace: testCase.current.Namespace}, after))
			if testCase.allowed {
				require.NoError(t, err)
				assert.Equal(t, int32(8065), after.Spec.Ports[0].Port)
			} else {
				require.Error(t, err)
				assert.Equal(t, int32(80), after.Spec.Ports[0].Port)
			}
		})
	}
}

func TestResourceHelper_Delete_Ownership(t *testing.T) {
	owner := newOwner("mm", "tenant", "owner-uid")
	logger := logr.Discard()
	ownedRefs := controllerRef("installation.mattermost.com/v1beta1", "Mattermost", "mm", "owner-uid")
	foreignRefs := controllerRef("installation.mattermost.com/v1beta1", "Mattermost", "other", "other-uid")

	t.Run("service", func(t *testing.T) {
		key := types.NamespacedName{Name: "svc", Namespace: "default"}
		for _, testCase := range []struct {
			description string
			refs        []metav1.OwnerReference
			deleted     bool
		}{
			{"owned", ownedRefs, true},
			{"foreign", foreignRefs, false},
			{"unowned", nil, false},
		} {
			t.Run(testCase.description, func(t *testing.T) {
				obj := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace, OwnerReferences: testCase.refs}}
				c := fake.NewClientBuilder().WithObjects(obj).Build()
				require.NoError(t, NewResourceHelper(c, scheme.Scheme).DeleteService(owner, key, logger))
				err := c.Get(context.TODO(), key, &corev1.Service{})
				assert.Equal(t, testCase.deleted, k8sErrors.IsNotFound(err))
			})
		}
	})

	t.Run("deployment", func(t *testing.T) {
		key := types.NamespacedName{Name: "deploy", Namespace: "default"}
		for _, testCase := range []struct {
			description string
			refs        []metav1.OwnerReference
			deleted     bool
		}{
			{"owned", ownedRefs, true},
			{"foreign", foreignRefs, false},
		} {
			t.Run(testCase.description, func(t *testing.T) {
				obj := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace, OwnerReferences: testCase.refs}}
				c := fake.NewClientBuilder().WithObjects(obj).Build()
				require.NoError(t, NewResourceHelper(c, scheme.Scheme).DeleteDeployment(owner, key, logger))
				err := c.Get(context.TODO(), key, &appsv1.Deployment{})
				assert.Equal(t, testCase.deleted, k8sErrors.IsNotFound(err))
			})
		}
	})

	t.Run("ingress", func(t *testing.T) {
		key := types.NamespacedName{Name: "ing", Namespace: "default"}
		for _, testCase := range []struct {
			description string
			refs        []metav1.OwnerReference
			deleted     bool
		}{
			{"owned", ownedRefs, true},
			{"foreign", foreignRefs, false},
		} {
			t.Run(testCase.description, func(t *testing.T) {
				obj := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace, OwnerReferences: testCase.refs}}
				c := fake.NewClientBuilder().WithObjects(obj).Build()
				require.NoError(t, NewResourceHelper(c, scheme.Scheme).DeleteIngress(owner, key, logger))
				err := c.Get(context.TODO(), key, &networkingv1.Ingress{})
				assert.Equal(t, testCase.deleted, k8sErrors.IsNotFound(err))
			})
		}
	})

	t.Run("ingress class", func(t *testing.T) {
		key := types.NamespacedName{Name: "nginx"}
		for _, testCase := range []struct {
			description string
			refs        []metav1.OwnerReference
			deleted     bool
		}{
			{"owned", ownedRefs, true},
			{"foreign shared class", nil, false},
		} {
			t.Run(testCase.description, func(t *testing.T) {
				obj := &networkingv1.IngressClass{ObjectMeta: metav1.ObjectMeta{Name: key.Name, OwnerReferences: testCase.refs}}
				c := fake.NewClientBuilder().WithObjects(obj).Build()
				require.NoError(t, NewResourceHelper(c, scheme.Scheme).DeleteIngressClass(owner, key, logger))
				err := c.Get(context.TODO(), key, &networkingv1.IngressClass{})
				assert.Equal(t, testCase.deleted, k8sErrors.IsNotFound(err))
			})
		}
	})
}
