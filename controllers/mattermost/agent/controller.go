package agent

import (
	"context"
	"reflect"
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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	healthCheckRequeueDelay = 6 * time.Second
	dependencyRequeueDelay  = 15 * time.Second
	configIssueRequeueDelay = 60 * time.Second

	// mattermostRefNameIndex indexes Agents by the Mattermost CR they reference.
	mattermostRefNameIndex = "spec.mattermostRef.name"
)

// errConfigIssue marks confirmed configuration gaps that the operator cannot
// fix by itself (externally-provisioned Secrets missing or malformed, gateway
// not configured on the installation). At the phase boundary Reconcile surfaces
// the message on the Agent status and requeues quietly; transient API errors
// are returned as real errors and get controller-runtime backoff.
var errConfigIssue = errors.New("configuration issue")

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
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &mmv1beta.Agent{}, mattermostRefNameIndex, indexAgentByMattermostRef)
	if err != nil {
		return errors.Wrap(err, "failed to index agents by mattermostRef name")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1beta.Agent{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Watches(&mmv1beta.Mattermost{}, handler.EnqueueRequestsFromMapFunc(r.agentsForMattermost)).
		Watches(&appsv1.Deployment{}, handler.EnqueueRequestsFromMapFunc(r.agentsForLiteLLMDeployment)).
		// Prerequisite Secrets (bot token, virtual key) are provisioned by
		// the agents plugin or the user, not owned by the Agent CR, so
		// Owns(&corev1.Secret{}) never fires for them; watch them explicitly
		// to converge promptly instead of waiting for the polling backstop.
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.agentsForSecret)).
		Complete(r)
}

func indexAgentByMattermostRef(obj client.Object) []string {
	agent, ok := obj.(*mmv1beta.Agent)
	if !ok {
		return nil
	}
	return []string{agent.Spec.MattermostRef.Name}
}

// agentsForMattermost maps a Mattermost event to the Agents referencing it, so
// installation readiness transitions enqueue agents promptly (the polling
// requeue remains as a backstop).
func (r *AgentReconciler) agentsForMattermost(ctx context.Context, obj client.Object) []reconcile.Request {
	agents := &mmv1beta.AgentList{}
	err := r.Client.List(ctx, agents,
		client.InNamespace(obj.GetNamespace()),
		client.MatchingFields{mattermostRefNameIndex: obj.GetName()},
	)
	if err != nil {
		r.Log.Error(err, "Failed to list Agents for Mattermost event", "mattermost", obj.GetName())
		return nil
	}

	requests := make([]reconcile.Request, 0, len(agents.Items))
	for _, agent := range agents.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace},
		})
	}
	return requests
}

// agentsForSecret maps a Secret event to the Agents in the same namespace
// that reference it as an unowned prerequisite: the plugin-provisioned bot
// token, the operator-managed-gateway virtual key, or an external gateway
// virtual key.
func (r *AgentReconciler) agentsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	agents := &mmv1beta.AgentList{}
	err := r.Client.List(ctx, agents, client.InNamespace(obj.GetNamespace()))
	if err != nil {
		r.Log.Error(err, "Failed to list Agents for Secret event", "secret", obj.GetName())
		return nil
	}

	var requests []reconcile.Request
	for i := range agents.Items {
		if !agentReferencesSecret(&agents.Items[i], obj.GetName()) {
			continue
		}
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: agents.Items[i].Name, Namespace: agents.Items[i].Namespace},
		})
	}
	return requests
}

// agentReferencesSecret reports whether the named Secret is one of the
// agent's unowned prerequisite Secrets.
func agentReferencesSecret(agent *mmv1beta.Agent, secretName string) bool {
	if agent.BotTokenSecretName() == secretName {
		return true
	}
	if agent.HasOperatorManagedGateway() && agent.LiteLLMKeySecretName() == secretName {
		return true
	}
	if agent.HasExternalGateway() && agent.Spec.LLMGateway.External.VirtualKeySecret == secretName {
		return true
	}
	return false
}

// agentsForLiteLLMDeployment maps LiteLLM gateway Deployment events to the
// operator-managed-gateway Agents in the same namespace, so gateway readiness
// transitions enqueue agents promptly.
func (r *AgentReconciler) agentsForLiteLLMDeployment(ctx context.Context, obj client.Object) []reconcile.Request {
	if obj.GetName() != mmv1beta.AgentLiteLLMDeploymentName {
		return nil
	}

	agents := &mmv1beta.AgentList{}
	err := r.Client.List(ctx, agents, client.InNamespace(obj.GetNamespace()))
	if err != nil {
		r.Log.Error(err, "Failed to list Agents for LiteLLM Deployment event", "namespace", obj.GetNamespace())
		return nil
	}

	var requests []reconcile.Request
	for _, agent := range agents.Items {
		if !agent.HasOperatorManagedGateway() {
			continue
		}
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace},
		})
	}
	return requests
}

// +kubebuilder:rbac:groups=installation.mattermost.com,resources=agents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=installation.mattermost.com,resources=agents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=installation.mattermost.com,resources=mattermosts,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;secrets;serviceaccounts;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
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

	// We copy status to not to refetch the resource.
	status := agent.Status
	// Indicate that the newest generation of the resource has been observed.
	status.ObservedGeneration = agent.Generation

	// Set a new Agent's state to reconciling.
	if len(agent.Status.State) == 0 {
		err = r.updateStatusReconciling(ctx, agent, status, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Set defaults and update the resource with said defaults if anything is
	// different.
	originalAgent := agent.DeepCopy()
	agent.SetDefaults()
	if !reflect.DeepEqual(originalAgent.Spec, agent.Spec) {
		agent.Status = status
		err = r.updateSpec(ctx, reqLogger, agent)
		if err != nil {
			r.updateStatusReconcilingAndLogError(ctx, originalAgent, status, reqLogger, err)
			return reconcile.Result{}, err
		}
	}

	// Gate on the referenced Mattermost installation being stable.
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
		return r.requeueWaitingForDependency(ctx, agent, status, reqLogger)
	}

	// Gate on the LiteLLM gateway, which is managed by the Mattermost controller.
	ready, err := r.checkLiteLLMGateway(ctx, agent, mm, reqLogger)
	if err != nil {
		return r.handleCheckError(ctx, agent, status, reqLogger, err)
	}
	if !ready {
		return r.requeueWaitingForDependency(ctx, agent, status, reqLogger)
	}

	err = r.checkAgent(ctx, agent, reqLogger)
	if err != nil {
		return r.handleCheckError(ctx, agent, status, reqLogger, err)
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

func (r *AgentReconciler) updateSpec(ctx context.Context, reqLogger logr.Logger, updated *mmv1beta.Agent) error {
	reqLogger.Info("Updating Agent spec")
	return r.Client.Update(ctx, updated)
}

// requeueWaitingForDependency persists an up-to-date Reconciling status
// (current ObservedGeneration, cleared Error, no stale Stable state) before
// requeueing to wait on a dependency. The status helper skips unchanged
// writes, so repeated waits are cheap.
func (r *AgentReconciler) requeueWaitingForDependency(ctx context.Context, agent *mmv1beta.Agent, status mmv1beta.AgentStatus, reqLogger logr.Logger) (ctrl.Result, error) {
	status.Error = ""
	err := r.updateStatusReconciling(ctx, agent, status, reqLogger)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{RequeueAfter: dependencyRequeueDelay}, nil
}

// handleCheckError surfaces err on the Agent status and maps confirmed config
// issues (errConfigIssue) to a quiet requeue; anything else propagates as a
// real error for controller-runtime backoff.
func (r *AgentReconciler) handleCheckError(ctx context.Context, agent *mmv1beta.Agent, status mmv1beta.AgentStatus, reqLogger logr.Logger, err error) (ctrl.Result, error) {
	r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
	if errors.Is(err, errConfigIssue) {
		return reconcile.Result{RequeueAfter: configIssueRequeueDelay}, nil
	}
	return reconcile.Result{}, err
}
