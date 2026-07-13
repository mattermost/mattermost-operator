package mattermost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// checkLiteLLM reconciles the operator-managed LiteLLM gateway
// (Deployment/Service owned by the Mattermost CR, plus an unowned master-key
// Secret) configured via spec.agents.llmGateway. It only creates/updates the
// gateway; the disable-transition cleanup runs earlier in Reconcile via
// cleanupLiteLLMIfDisabled so it never waits on server health.
func (r *MattermostReconciler) checkLiteLLM(ctx context.Context, mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	if !mattermost.OperatorManagedLLMGatewayEnabled() {
		return nil
	}

	reqLogger = reqLogger.WithValues("Reconcile", "litellm")

	dbCredentialsHash, err := r.checkLiteLLMDBCredentials(ctx, mattermost)
	if err != nil {
		return err
	}

	err = r.checkLiteLLMMasterKey(ctx, mattermost, reqLogger)
	if err != nil {
		return err
	}

	err = r.checkLiteLLMDeployment(ctx, mattermost, dbCredentialsHash, reqLogger)
	if err != nil {
		return err
	}

	return r.checkLiteLLMService(ctx, mattermost, reqLogger)
}

// cleanupLiteLLMIfDisabled removes the LiteLLM gateway when the Mattermost CR
// does not (or no longer does) enable it. It runs early in Reconcile, before
// the server health gate, so a broken server cannot strand a disabled
// gateway. The deletes are ownership-checked, so gateways belonging to other
// Mattermost CRs in the same namespace are left alone.
func (r *MattermostReconciler) cleanupLiteLLMIfDisabled(ctx context.Context, mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	if mattermost.OperatorManagedLLMGatewayEnabled() {
		return nil
	}

	return r.deleteLiteLLMResources(ctx, mattermost, reqLogger.WithValues("Reconcile", "litellm"))
}

// checkLiteLLMDBCredentials verifies that the externally-provisioned
// PostgreSQL connection-string Secret exists and is populated, and returns a
// short hash of the connection string so rotation rolls the LiteLLM pods. The
// operator never creates this Secret.
func (r *MattermostReconciler) checkLiteLLMDBCredentials(ctx context.Context, mattermost *mmv1beta.Mattermost) (string, error) {
	secret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDBCredentialsSecret,
		Namespace: mattermost.Namespace,
	}, secret)
	if k8sErrors.IsNotFound(err) {
		return "", fmt.Errorf("required Secret %q (key %q) not found in namespace %s; the operator-managed LiteLLM gateway requires an externally-provisioned PostgreSQL connection string",
			mmv1beta.AgentLiteLLMDBCredentialsSecret, mmv1beta.SecretKeyConnectionString, mattermost.Namespace)
	}
	if err != nil {
		return "", errors.Wrap(err, "failed to check litellm db credentials secret")
	}

	connectionString := secret.Data[mmv1beta.SecretKeyConnectionString]
	if len(connectionString) == 0 {
		return "", fmt.Errorf("secret %q must contain a non-empty %q key with a PostgreSQL connection string",
			mmv1beta.AgentLiteLLMDBCredentialsSecret, mmv1beta.SecretKeyConnectionString)
	}

	return shortHash(connectionString), nil
}

// shortHash returns a short, stable digest of value for use in annotations.
func shortHash(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:8])
}

// checkLiteLLMMasterKey ensures the LiteLLM master-key Secret exists,
// generating one if necessary. Randomness is only generated after a Get
// confirms the Secret is missing. The Secret is created without an owner
// reference and is retained when the gateway is disabled: virtual keys hashed
// with it live in the external LiteLLM database, so deleting it would
// invalidate them on re-enable.
func (r *MattermostReconciler) checkLiteLLMMasterKey(ctx context.Context, mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	existing := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
		Namespace: mattermost.Namespace,
	}, existing)
	if err == nil {
		if len(existing.Data[mmv1beta.SecretKeyMasterKey]) == 0 {
			return fmt.Errorf("secret %q must contain a non-empty %q key",
				mmv1beta.AgentLiteLLMMasterKeySecretName, mmv1beta.SecretKeyMasterKey)
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

	desired := mattermostApp.GenerateLiteLLMMasterKeySecret(mattermost, "sk-"+hexKey)
	return r.Resources.CreateUnowned(desired, reqLogger)
}

func (r *MattermostReconciler) checkLiteLLMDeployment(ctx context.Context, mattermost *mmv1beta.Mattermost, dbCredentialsHash string, reqLogger logr.Logger) error {
	desired := mattermostApp.GenerateLiteLLMDeployment(mattermost, dbCredentialsHash)

	err := r.Resources.CreateDeploymentIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to create litellm deployment")
	}

	current := &appsv1.Deployment{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return errors.Wrap(err, "failed to get litellm deployment")
	}

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *MattermostReconciler) checkLiteLLMService(ctx context.Context, mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	desired := mattermostApp.GenerateLiteLLMService(mattermost)

	err := r.Resources.CreateServiceIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to create litellm service")
	}

	current := &corev1.Service{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return errors.Wrap(err, "failed to get litellm service")
	}

	resources.CopyServiceEmptyAutoAssignedFields(desired, current)

	return r.Resources.Update(current, desired, reqLogger)
}

// deleteLiteLLMResources removes the LiteLLM Deployment and Service when the
// gateway is disabled. Objects are only deleted when this Mattermost CR
// controls them: gateway names are fixed, so another installation in the same
// namespace may own same-named objects. The master-key Secret is deliberately
// retained (see checkLiteLLMMasterKey).
func (r *MattermostReconciler) deleteLiteLLMResources(ctx context.Context, mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	err := r.deleteLiteLLMObjectIfOwned(ctx, mattermost, &appsv1.Deployment{}, mmv1beta.AgentLiteLLMDeploymentName, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to delete disabled litellm deployment")
	}

	err = r.deleteLiteLLMObjectIfOwned(ctx, mattermost, &corev1.Service{}, mmv1beta.AgentLiteLLMServiceName, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to delete disabled litellm service")
	}

	return nil
}

// deleteLiteLLMObjectIfOwned deletes the named object only when it exists and
// is controlled by the given Mattermost CR; not-found and not-owned are clean
// no-ops.
func (r *MattermostReconciler) deleteLiteLLMObjectIfOwned(ctx context.Context, mattermost *mmv1beta.Mattermost, obj client.Object, name string, reqLogger logr.Logger) error {
	err := r.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: mattermost.Namespace}, obj)
	if k8sErrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "failed to check %T %s", obj, name)
	}

	if !metav1.IsControlledBy(obj, mattermost) {
		return nil
	}

	reqLogger.Info("Deleting resource", "kind", fmt.Sprintf("%T", obj), "name", name)
	err = r.Client.Delete(ctx, obj)
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete %T %s", obj, name)
	}

	return nil
}

// handleLiteLLMError surfaces a gateway reconcile failure without knocking
// the (already healthy) installation out of Stable: the error is recorded on
// status.Error, the Stable status is persisted, and the reconcile is retried
// after a delay. Agents that need the gateway independently gate on gateway
// deployment readiness, so Stable-with-broken-gateway is self-consistent.
func (r *MattermostReconciler) handleLiteLLMError(mattermost *mmv1beta.Mattermost, status mmv1beta.MattermostStatus, reqLogger logr.Logger, gatewayErr error) (reconcile.Result, error) {
	status.Error = gatewayErr.Error()
	err := r.updateStatus(mattermost, status, reqLogger)
	if err != nil {
		reqLogger.Error(err, "Error updating status")
	}
	reqLogger.Info("LiteLLM gateway not reconciled", "msg", gatewayErr.Error())
	return reconcile.Result{RequeueAfter: healthCheckRequeueDelay}, nil
}
