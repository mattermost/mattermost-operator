package mattermost

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	"github.com/mattermost/mattermost-operator/pkg/resources"
	"github.com/mattermost/mattermost-operator/pkg/utils"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// checkLiteLLM reconciles the operator-managed LiteLLM gateway
// (Deployment/Service owned by the Mattermost CR, plus an unowned master-key
// Secret) configured via spec.agents.llmGateway.
func (r *MattermostReconciler) checkLiteLLM(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	reqLogger = reqLogger.WithValues("Reconcile", "litellm")

	if !mattermost.OperatorManagedLLMGatewayEnabled() {
		return r.deleteLiteLLMResources(mattermost, reqLogger)
	}

	err := r.checkLiteLLMDBCredentials(mattermost)
	if err != nil {
		return err
	}

	err = r.checkLiteLLMMasterKey(mattermost, reqLogger)
	if err != nil {
		return err
	}

	err = r.checkLiteLLMDeployment(mattermost, reqLogger)
	if err != nil {
		return err
	}

	return r.checkLiteLLMService(mattermost, reqLogger)
}

// checkLiteLLMDBCredentials verifies that the externally-provisioned PostgreSQL
// connection-string Secret exists and is populated. The operator never creates
// this Secret.
func (r *MattermostReconciler) checkLiteLLMDBCredentials(mattermost *mmv1beta.Mattermost) error {
	secret := &corev1.Secret{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDBCredentialsSecret,
		Namespace: mattermost.Namespace,
	}, secret)
	if k8sErrors.IsNotFound(err) {
		return fmt.Errorf("required Secret %q (key %q) not found in namespace %s; the operator-managed LiteLLM gateway requires an externally-provisioned PostgreSQL connection string",
			mmv1beta.AgentLiteLLMDBCredentialsSecret, "connectionString", mattermost.Namespace)
	}
	if err != nil {
		return errors.Wrap(err, "failed to check litellm db credentials secret")
	}

	if len(secret.Data["connectionString"]) == 0 {
		return fmt.Errorf("secret %q must contain a non-empty %q key with a PostgreSQL connection string",
			mmv1beta.AgentLiteLLMDBCredentialsSecret, "connectionString")
	}

	return nil
}

// checkLiteLLMMasterKey ensures the LiteLLM master-key Secret exists, generating
// one if necessary. The Secret is created without an owner reference and is
// retained when the gateway is disabled: virtual keys hashed with it live in
// the external LiteLLM database, so deleting it would invalidate them on
// re-enable.
func (r *MattermostReconciler) checkLiteLLMMasterKey(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	existing := &corev1.Secret{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
		Namespace: mattermost.Namespace,
	}, existing)
	if err == nil {
		if len(existing.Data["masterKey"]) == 0 {
			return fmt.Errorf("secret %q must contain a non-empty %q key",
				mmv1beta.AgentLiteLLMMasterKeySecretName, "masterKey")
		}
		return nil
	}
	if !k8sErrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to check litellm master key secret")
	}

	hexKey, err := utils.RandomHex(32)
	if err != nil {
		return errors.Wrap(err, "failed to generate litellm master key")
	}

	desired := mattermostApp.GenerateLiteLLMMasterKeySecret(mattermost.Namespace, "sk-"+hexKey)
	return r.Resources.CreateIfNotExists(nil, desired, reqLogger)
}

func (r *MattermostReconciler) checkLiteLLMDeployment(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	desired := mattermostApp.GenerateLiteLLMDeployment(mattermost)

	err := r.Resources.CreateDeploymentIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to create litellm deployment")
	}

	current := &appsv1.Deployment{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return errors.Wrap(err, "failed to get litellm deployment")
	}

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *MattermostReconciler) checkLiteLLMService(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	desired := mattermostApp.GenerateLiteLLMService(mattermost)

	err := r.Resources.CreateServiceIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to create litellm service")
	}

	current := &corev1.Service{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return errors.Wrap(err, "failed to get litellm service")
	}

	resources.CopyServiceEmptyAutoAssignedFields(desired, current)

	return r.Resources.Update(current, desired, reqLogger)
}

// deleteLiteLLMResources removes the LiteLLM Deployment and Service when the
// gateway is disabled. The master-key Secret is deliberately retained (see
// checkLiteLLMMasterKey).
func (r *MattermostReconciler) deleteLiteLLMResources(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	err := r.Resources.DeleteDeployment(types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDeploymentName,
		Namespace: mattermost.Namespace,
	}, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to delete disabled litellm deployment")
	}

	err = r.Resources.DeleteService(types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMServiceName,
		Namespace: mattermost.Namespace,
	}, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to delete disabled litellm service")
	}

	return nil
}
