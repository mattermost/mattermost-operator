// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	AgentEgressPolicyDeny     AgentEgressPolicy = "deny"
	AgentEgressPolicyAllowWeb AgentEgressPolicy = "allowWeb"
	AgentEgressPolicyAllow    AgentEgressPolicy = "allow"

	AgentAppName                    = "mattermost-agent"
	AgentContainerName              = "agent"
	AgentHTTPPort                   = int32(8080)
	AgentLiteLLMDefaultImage        = "ghcr.io/berriai/litellm-database:main-v1.82.0-stable"
	AgentLiteLLMPort                = int32(4000)
	AgentLiteLLMDeploymentName      = "mm-agent-litellm"
	AgentLiteLLMServiceName         = "mm-agent-litellm"
	AgentLiteLLMMasterKeySecretName = "mm-agent-litellm-master-key"
	AgentLiteLLMDBCredentialsSecret = "mm-agent-litellm-db-credentials"
	AgentStorageDefaultMountPath    = "/data"

	SecretKeyBotToken         = "token"
	SecretKeyHookSecret       = "hookSecret"
	SecretKeyAPIKey           = "apiKey"
	SecretKeyMasterKey        = "masterKey"
	SecretKeyConnectionString = "connectionString"
)

// SetDefaults sets missing values in the Agent manifest to their defaults.
func (a *Agent) SetDefaults() {
	if a.Spec.EgressPolicy == "" {
		a.Spec.EgressPolicy = AgentEgressPolicyDeny
	}

	if a.Spec.Resources.Requests == nil && a.Spec.Resources.Limits == nil {
		a.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		}
		a.Spec.Resources.Limits = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		}
	}

	if a.Spec.Storage != nil && a.Spec.Storage.MountPath == "" {
		a.Spec.Storage.MountPath = AgentStorageDefaultMountPath
	}
}

// HasOperatorManagedGateway reports whether the agent opts into the
// operator-managed LiteLLM gateway of its Mattermost installation.
func (a *Agent) HasOperatorManagedGateway() bool {
	return a.Spec.LLMGateway != nil && a.Spec.LLMGateway.OperatorManaged != nil
}

// HasExternalGateway reports whether the agent uses an externally managed gateway.
func (a *Agent) HasExternalGateway() bool {
	return a.Spec.LLMGateway != nil && a.Spec.LLMGateway.External != nil
}

// GatewayEndpoint resolves the configured gateway URL and virtual-key Secret.
func (a *Agent) GatewayEndpoint() (baseURL, keySecretName string, ok bool) {
	switch {
	case a.HasOperatorManagedGateway():
		return LiteLLMServiceURL(a.Namespace), a.LiteLLMKeySecretName(), true
	case a.HasExternalGateway():
		return a.Spec.LLMGateway.External.URL, a.Spec.LLMGateway.External.VirtualKeySecret, true
	default:
		return "", "", false
	}
}

// ClusterServiceURL returns the HTTP URL for an in-cluster Service.
func ClusterServiceURL(name, namespace string, port int32) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", name, namespace, port)
}

// LiteLLMServiceURL returns the in-cluster base URL for the LiteLLM service.
func LiteLLMServiceURL(namespace string) string {
	return ClusterServiceURL(AgentLiteLLMServiceName, namespace, AgentLiteLLMPort)
}

// AgentLabels returns the full set of labels for all resources belonging to the agent.
func AgentLabels(agent *Agent) map[string]string {
	l := AgentResourceLabels(agent.Name)
	l[AgentNameLabel] = agent.Name
	l[ClusterLabel] = agent.Spec.MattermostRef.Name
	l["app"] = AgentAppName
	return l
}

// AgentSelectorLabels returns the minimal label set used as the pod selector.
// Selector labels are immutable on a Deployment after creation; keeping this set
// minimal means future additions to AgentLabels do not break the selector.
func AgentSelectorLabels(agent *Agent) map[string]string {
	return map[string]string{
		AgentNameLabel: agent.Name,
		"app":          AgentAppName,
	}
}

// AgentResourceLabels returns the resource-scoped label for the agent.
func AgentResourceLabels(name string) map[string]string {
	return map[string]string{ClusterResourceLabel: name}
}

func (a *Agent) scopedName(suffix string) string {
	return "agent-" + a.Name + "-" + suffix
}

// BotTokenSecretName returns the name of the K8s Secret storing the agent's bot token.
func (a *Agent) BotTokenSecretName() string {
	return a.scopedName("token")
}

// LiteLLMKeySecretName returns the name of the K8s Secret storing this agent's LiteLLM virtual key.
func (a *Agent) LiteLLMKeySecretName() string {
	return a.scopedName("litellm-key")
}

// HookSecretName returns the name of the K8s Secret storing this agent's hook secret.
func (a *Agent) HookSecretName() string {
	return a.scopedName("hook-secret")
}

// StoragePVCName returns the name of the PVC for the agent's persistent storage.
func (a *Agent) StoragePVCName() string {
	return a.scopedName("storage")
}
