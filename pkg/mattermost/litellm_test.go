package mattermost

import (
	"fmt"
	"testing"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// testMattermostWithGateway returns a Mattermost CR with the LiteLLM gateway
// enabled and defaults applied.
func testMattermostWithGateway(t *testing.T, name, ns string) *mmv1beta.Mattermost {
	mm := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: mmv1beta.MattermostSpec{
			IngressName: "test.example.com",
			Agents: &mmv1beta.MattermostAgents{
				Enabled:    true,
				LLMGateway: &mmv1beta.AgentsLLMGateway{},
			},
		},
	}
	require.NoError(t, mm.SetDefaults())
	return mm
}

func TestGenerateLiteLLMMasterKeySecret(t *testing.T) {
	mm := testMattermostWithGateway(t, "mm-test", "my-namespace")
	secret := GenerateLiteLLMMasterKeySecret(mm, "sk-test-key")

	assert.Equal(t, mmv1beta.AgentLiteLLMMasterKeySecretName, secret.Name)
	assert.Equal(t, "my-namespace", secret.Namespace)
	assert.Equal(t, LiteLLMLabels(mm), secret.Labels)
	assert.Equal(t, []byte("sk-test-key"), secret.Data[mmv1beta.SecretKeyMasterKey])
	assert.Empty(t, secret.OwnerReferences)
}

func TestGenerateLiteLLMDeployment(t *testing.T) {
	mm := testMattermostWithGateway(t, "mm-test", "my-namespace")
	dep := GenerateLiteLLMDeployment(mm)

	assert.Equal(t, mmv1beta.AgentLiteLLMDeploymentName, dep.Name)
	assert.Equal(t, "my-namespace", dep.Namespace)
	assert.Equal(t, LiteLLMLabels(mm), dep.Labels)
	assert.Equal(t, "mm-test", dep.Labels[mmv1beta.ClusterLabel])

	require.NotNil(t, dep.Spec.Replicas)
	assert.Equal(t, int32(1), *dep.Spec.Replicas)

	assert.Equal(t, LiteLLMSelectorLabels(), dep.Spec.Selector.MatchLabels)
	assert.NotContains(t, dep.Spec.Selector.MatchLabels, mmv1beta.ClusterLabel)

	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	c := dep.Spec.Template.Spec.Containers[0]

	assert.Equal(t, "litellm", c.Name)
	assert.Equal(t, mmv1beta.AgentLiteLLMDefaultImage, c.Image)
	assert.Empty(t, c.Args)

	envMap := envVarsByName(c.Env)

	require.Contains(t, envMap, "DATABASE_URL")
	require.NotNil(t, envMap["DATABASE_URL"].ValueFrom)
	require.NotNil(t, envMap["DATABASE_URL"].ValueFrom.SecretKeyRef)
	assert.Equal(t, mmv1beta.AgentLiteLLMDBCredentialsSecret, envMap["DATABASE_URL"].ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, mmv1beta.SecretKeyConnectionString, envMap["DATABASE_URL"].ValueFrom.SecretKeyRef.Key)

	require.Contains(t, envMap, "LITELLM_MASTER_KEY")
	require.NotNil(t, envMap["LITELLM_MASTER_KEY"].ValueFrom)
	require.NotNil(t, envMap["LITELLM_MASTER_KEY"].ValueFrom.SecretKeyRef)
	assert.Equal(t, mmv1beta.AgentLiteLLMMasterKeySecretName, envMap["LITELLM_MASTER_KEY"].ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, mmv1beta.SecretKeyMasterKey, envMap["LITELLM_MASTER_KEY"].ValueFrom.SecretKeyRef.Key)

	require.Contains(t, envMap, "STORE_MODEL_IN_DB")
	assert.Equal(t, "True", envMap["STORE_MODEL_IN_DB"].Value)

	require.Len(t, c.Ports, 1)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, c.Ports[0].ContainerPort)
	assert.Equal(t, "http", c.Ports[0].Name)

	// Default resources applied by SetDefaults.
	assert.Equal(t, resource.MustParse("500m"), c.Resources.Requests[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("512Mi"), c.Resources.Requests[corev1.ResourceMemory])
	assert.Equal(t, resource.MustParse("2"), c.Resources.Limits[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("2Gi"), c.Resources.Limits[corev1.ResourceMemory])

	require.NotNil(t, c.LivenessProbe)
	require.NotNil(t, c.LivenessProbe.HTTPGet)
	assert.Equal(t, "/health/liveliness", c.LivenessProbe.HTTPGet.Path)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, c.LivenessProbe.HTTPGet.Port.IntVal)

	require.NotNil(t, c.ReadinessProbe)
	require.NotNil(t, c.ReadinessProbe.HTTPGet)
	assert.Equal(t, "/health/readiness", c.ReadinessProbe.HTTPGet.Path)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, c.ReadinessProbe.HTTPGet.Port.IntVal)
}

func TestGenerateLiteLLMDeployment_CustomImageAndResources(t *testing.T) {
	mm := testMattermostWithGateway(t, "mm-test", "my-namespace")
	mm.Spec.Agents.LLMGateway.Image = "ghcr.io/berriai/litellm-database:v1.99.9"
	mm.Spec.Agents.LLMGateway.Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("250m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
	}

	dep := GenerateLiteLLMDeployment(mm)

	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	c := dep.Spec.Template.Spec.Containers[0]

	assert.Equal(t, "ghcr.io/berriai/litellm-database:v1.99.9", c.Image)
	assert.Equal(t, corev1.PullIfNotPresent, c.ImagePullPolicy)
	assert.Equal(t, resource.MustParse("250m"), c.Resources.Requests[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("256Mi"), c.Resources.Requests[corev1.ResourceMemory])
	assert.Equal(t, resource.MustParse("1"), c.Resources.Limits[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("1Gi"), c.Resources.Limits[corev1.ResourceMemory])
}

func TestGenerateLiteLLMDeployment_MutableTagAlwaysPulls(t *testing.T) {
	mm := testMattermostWithGateway(t, "mm-test", "my-namespace")
	mm.Spec.Agents.LLMGateway.Image = "ghcr.io/berriai/litellm-database:latest"

	dep := GenerateLiteLLMDeployment(mm)

	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, corev1.PullAlways, dep.Spec.Template.Spec.Containers[0].ImagePullPolicy)
}

func TestGenerateLiteLLMService(t *testing.T) {
	mm := testMattermostWithGateway(t, "mm-test", "my-namespace")
	svc := GenerateLiteLLMService(mm)

	assert.Equal(t, mmv1beta.AgentLiteLLMServiceName, svc.Name)
	assert.Equal(t, "my-namespace", svc.Namespace)
	assert.Equal(t, LiteLLMLabels(mm), svc.Labels)

	assert.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type)
	assert.Equal(t, LiteLLMSelectorLabels(), svc.Spec.Selector)

	require.Len(t, svc.Spec.Ports, 1)
	port := svc.Spec.Ports[0]
	assert.Equal(t, "http", port.Name)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, port.Port)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, port.TargetPort.IntVal)
}

func TestLiteLLMServiceURL(t *testing.T) {
	url := mmv1beta.LiteLLMServiceURL("my-namespace")
	expected := fmt.Sprintf(
		"http://%s.my-namespace.svc.cluster.local:%d",
		mmv1beta.AgentLiteLLMServiceName,
		mmv1beta.AgentLiteLLMPort,
	)
	assert.Equal(t, expected, url)
}
