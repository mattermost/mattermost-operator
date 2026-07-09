package agent

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/mattermost-operator/pkg/resources"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	healthCheckRequeueDelay = 6 * time.Second
	dependencyRequeueDelay  = 15 * time.Second
	configIssueRequeueDelay = 60 * time.Second

	agentFinalizer = "agent.installation.mattermost.com/finalizer"
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
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Complete(r)
}

// +kubebuilder:rbac:groups=installation.mattermost.com,resources=agents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=installation.mattermost.com,resources=agents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=installation.mattermost.com,resources=agents/finalizers,verbs=update
// +kubebuilder:rbac:groups=installation.mattermost.com,resources=mattermosts,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;secrets;serviceaccounts;configmaps;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

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

	// Handle finalizer / deletion.
	if agent.DeletionTimestamp.IsZero() {
		if agent.HasOperatorManagedGateway() {
			if !controllerutil.ContainsFinalizer(agent, agentFinalizer) {
				controllerutil.AddFinalizer(agent, agentFinalizer)
				if err := r.Update(ctx, agent); err != nil {
					return reconcile.Result{}, errors.Wrap(err, "failed to add finalizer")
				}
				// The Update triggers a watch event that re-fires Reconcile with
				// the fresh object; no explicit requeue needed.
				return reconcile.Result{}, nil
			}
		}
	} else {
		if controllerutil.ContainsFinalizer(agent, agentFinalizer) {
			if err := r.cleanupLiteLLMIfLast(ctx, agent, reqLogger); err != nil {
				return reconcile.Result{}, err
			}
			controllerutil.RemoveFinalizer(agent, agentFinalizer)
			if err := r.Update(ctx, agent); err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
			}
		}
		return reconcile.Result{}, nil
	}

	status := agent.Status

	// Set initial state to Reconciling.
	if len(agent.Status.State) == 0 {
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
		return reconcile.Result{RequeueAfter: dependencyRequeueDelay}, nil
	}

	// LiteLLM gateway (operator-managed).
	if agent.HasOperatorManagedGateway() {
		if err = r.checkLiteLLMDBCredentials(ctx, agent); err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{RequeueAfter: configIssueRequeueDelay}, nil
		}
		if err = r.checkLiteLLMMasterKey(ctx, agent, reqLogger); err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}
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
			return reconcile.Result{RequeueAfter: dependencyRequeueDelay}, nil
		}
	}

	err = r.checkAgentServiceAccount(ctx, agent, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	err = r.checkHookSecret(ctx, agent, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	err = r.checkAgentService(ctx, agent, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	// PVC (must exist before Deployment references it)
	err = r.checkAgentPVC(ctx, agent, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	// Bot token / LiteLLM virtual-key Secrets are provisioned by the agents plugin,
	// not the operator. Check after LiteLLM is ready so the plugin can mint keys.
	if err = r.checkExternallyProvisionedSecrets(ctx, agent); err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{RequeueAfter: configIssueRequeueDelay}, nil
	}

	err = r.checkAgentDeployment(ctx, agent, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	err = r.checkAgentNetworkPolicy(ctx, agent, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	status, err = r.checkAgentHealth(ctx, agent, status, reqLogger)
	if err != nil {
		status.Error = err.Error()
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
