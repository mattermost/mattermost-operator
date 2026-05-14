// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	AgentEgressPolicyDeny           = "deny"
	AgentEgressPolicyAllowList      = "allowList"
	AgentEgressPolicyAllow          = "allow"
	AgentContainerName              = "agent"
	AgentHTTPPort                   = int32(8080)
	AgentBotTokenSecretNamePrefix   = "agent-"
	AgentLiteLLMDefaultImage        = "ghcr.io/berriai/litellm-database:main-v1.82.0-stable"
	AgentLiteLLMPort                = int32(4000)
	AgentLiteLLMDeploymentName      = "litellm"
	AgentLiteLLMServiceName         = "litellm"
	AgentLiteLLMConfigMapName       = "litellm-config"
	AgentLiteLLMMasterKeySecretName = "litellm-master-key"
	AgentLiteLLMDBCredentialsSecret = "litellm-db-credentials"
	AgentStorageDefaultMountPath    = "/data"
)

// Agent lifecycle phases (written to AgentStatus.Phase).
const (
	AgentPhaseProvisioning = "Provisioning"
	AgentPhaseDeploying    = "Deploying"
	AgentPhaseReady        = "Ready"
	AgentPhaseError        = "Error"
)

// SetDefaults sets missing values in the Agent manifest to their defaults.
func (a *Agent) SetDefaults() error {
	if a.Spec.EgressPolicy == "" {
		a.Spec.EgressPolicy = AgentEgressPolicyDeny
	}

	if a.Spec.Resources.Requests == nil {
		a.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		}
	}

	if a.Spec.Resources.Limits == nil {
		a.Spec.Resources.Limits = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		}
	}

	if a.Spec.LLMGateway != nil && a.Spec.LLMGateway.OperatorManaged != nil {
		if a.Spec.LLMGateway.OperatorManaged.Image == "" {
			a.Spec.LLMGateway.OperatorManaged.Image = AgentLiteLLMDefaultImage
		}
	}

	if a.Spec.Storage != nil && a.Spec.Storage.MountPath == "" {
		a.Spec.Storage.MountPath = AgentStorageDefaultMountPath
	}

	return nil
}

// AgentLabels returns the full set of labels for all resources belonging to the agent.
func AgentLabels(name string) map[string]string {
	l := AgentResourceLabels(name)
	l[ClusterLabel] = name
	l["app"] = AgentContainerName
	return l
}

// AgentSelectorLabels returns labels used as the pod selector.
func AgentSelectorLabels(name string) map[string]string {
	l := AgentResourceLabels(name)
	l[ClusterLabel] = name
	l["app"] = AgentContainerName
	return l
}

// AgentResourceLabels returns the resource-scoped label for the agent.
func AgentResourceLabels(name string) map[string]string {
	return map[string]string{ClusterResourceLabel: name}
}

// BotTokenSecretName returns the name of the K8s Secret storing the agent's bot token.
func (a *Agent) BotTokenSecretName() string {
	return AgentBotTokenSecretNamePrefix + a.Name + "-token"
}

// LiteLLMKeySecretName returns the name of the K8s Secret storing this agent's LiteLLM virtual key.
func (a *Agent) LiteLLMKeySecretName() string {
	return "agent-" + a.Name + "-litellm-key"
}

// HookSecretName returns the name of the K8s Secret storing this agent's hook secret.
func (a *Agent) HookSecretName() string {
	return "agent-" + a.Name + "-hook-secret"
}

// StoragePVCName returns the name of the PVC for the agent's persistent storage.
func (a *Agent) StoragePVCName() string {
	return a.Name + "-storage"
}
