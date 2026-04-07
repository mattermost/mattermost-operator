package agent

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/mattermost-operator/pkg/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	healthCheckRequeueDelay   = 6 * time.Second
	mattermostNotReadyDelay   = 15 * time.Second
)

// AgentReconciler reconciles an Agent object.
type AgentReconciler struct {
	client.Client
	Log       logr.Logger
	Scheme    *runtime.Scheme
	Resources *resources.ResourceHelper
}

func NewAgentReconciler(mgr ctrl.Manager) *AgentReconciler {
	return &AgentReconciler{
		Client:    mgr.GetClient(),
		Log:       ctrl.Log.WithName("controllers").WithName("Agent"),
		Scheme:    mgr.GetScheme(),
		Resources: resources.NewResourceHelper(mgr.GetClient(), mgr.GetScheme()),
	}
}

func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1beta.Agent{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Complete(r)
}

func (r *AgentReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Agent")

	// Fetch the Agent CR.
	agent := &mmv1beta.Agent{}
	err := r.Client.Get(ctx, request.NamespacedName, agent)
	if err != nil && k8sErrors.IsNotFound(err) {
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	}

	status := agent.Status

	// Set initial state to Reconciling.
	if len(agent.Status.State) == 0 {
		status.Phase = mmv1beta.AgentPhaseProvisioning
		err = r.updateStatusReconciling(ctx, agent, status, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Apply defaults.
	err = agent.SetDefaults()
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	// Check Mattermost CR readiness.
	mm := &mmv1beta.Mattermost{}
	err = r.Client.Get(ctx, types.NamespacedName{
		Name:      agent.Spec.MattermostRef.Name,
		Namespace: agent.Namespace,
	}, mm)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}
	if mm.Status.State != mmv1beta.Stable {
		reqLogger.Info("Mattermost not stable, requeuing", "mmState", mm.Status.State)
		return reconcile.Result{RequeueAfter: mattermostNotReadyDelay}, nil
	}

	// LiteLLM gateway (operator-managed).
	if agent.Spec.LLMGateway != nil && agent.Spec.LLMGateway.OperatorManaged != nil {
		if err = r.checkLiteLLMDeployment(ctx, agent, reqLogger); err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}
		if err = r.checkLiteLLMService(ctx, agent, reqLogger); err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}

		ready, err := r.checkLiteLLMReady(ctx, agent, reqLogger)
		if err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}
		if !ready {
			return reconcile.Result{RequeueAfter: mattermostNotReadyDelay}, nil
		}
	}

	// LiteLLM is ready (or not configured); transition to Deploying.
	if status.Phase == mmv1beta.AgentPhaseProvisioning {
		status.Phase = mmv1beta.AgentPhaseDeploying
	}

	// ServiceAccount
	err = r.checkAgentServiceAccount(ctx, agent, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	// Hook Secret
	err = r.checkHookSecret(ctx, agent, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	// Service
	err = r.checkAgentService(ctx, agent, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	// Deployment
	err = r.checkAgentDeployment(ctx, agent, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	// NetworkPolicy
	err = r.checkAgentNetworkPolicy(ctx, agent, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	// Health check
	status, err = r.checkAgentHealth(ctx, agent, status, reqLogger)
	if err != nil {
		statusErr := r.updateStatus(ctx, agent, status, reqLogger)
		if statusErr != nil {
			reqLogger.Error(statusErr, "Error updating status")
		}
		reqLogger.Info("Agent not healthy", "msg", err.Error())
		return reconcile.Result{RequeueAfter: healthCheckRequeueDelay}, nil
	}

	err = r.updateStatus(ctx, agent, status, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
