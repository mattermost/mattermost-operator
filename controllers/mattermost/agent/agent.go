package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	"github.com/mattermost/mattermost-operator/pkg/resources"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

type mmBot struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

type mmToken struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

// checkAgentBot provisions a bot account on the Mattermost server if needed.
// It uses the existence of the bot token secret as an idempotency check.
func (r *AgentReconciler) checkAgentBot(ctx context.Context, agent *mmv1beta.Agent, adminToken string, reqLogger logr.Logger) error {
	mmURL := "http://" + agent.Spec.MattermostRef.Name + "." + agent.Namespace + ".svc.cluster.local:8065"
	return r.checkAgentBotWithURL(ctx, agent, adminToken, mmURL, reqLogger)
}

// checkAgentBotWithURL is the testable inner function that accepts a configurable MM base URL.
func (r *AgentReconciler) checkAgentBotWithURL(ctx context.Context, agent *mmv1beta.Agent, adminToken, mmURL string, reqLogger logr.Logger) error {
	// Idempotency: if bot token secret exists, skip API calls entirely.
	existingSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      agent.BotTokenSecretName(),
		Namespace: agent.Namespace,
	}, existingSecret)
	if err == nil {
		reqLogger.Info("Bot token secret already exists, skipping bot provisioning")
		return nil
	}
	if !k8sErrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to check bot token secret")
	}

	reqLogger.Info("Provisioning bot account", "username", agent.Name)

	// Find existing bot by listing bots.
	var botUserID string
	bots, err := r.listBots(mmURL, adminToken)
	if err != nil {
		return errors.Wrap(err, "failed to list bots")
	}
	for _, bot := range bots {
		if bot.Username == agent.Name {
			botUserID = bot.UserID
			break
		}
	}

	// Create bot if not found.
	if botUserID == "" {
		bot, err := r.createBot(mmURL, adminToken, agent.Name)
		if err != nil {
			return errors.Wrap(err, "failed to create bot")
		}
		botUserID = bot.UserID
	}

	// Create access token.
	token, err := r.createBotToken(mmURL, adminToken, botUserID)
	if err != nil {
		return errors.Wrap(err, "failed to create bot token")
	}

	// Create K8s secret with the bot token.
	desired := mattermostApp.GenerateAgentBotTokenSecret(agent, token.Token)
	err = r.Resources.CreateSecretIfNotExists(agent, desired, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to create bot token secret")
	}

	return nil
}

func (r *AgentReconciler) listBots(mmURL, adminToken string) ([]mmBot, error) {
	req, err := http.NewRequest("GET", mmURL+"/api/v4/bots", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list bots returned status %d: %s", resp.StatusCode, string(body))
	}

	var bots []mmBot
	if err := json.NewDecoder(resp.Body).Decode(&bots); err != nil {
		return nil, err
	}
	return bots, nil
}

func (r *AgentReconciler) createBot(mmURL, adminToken, username string) (*mmBot, error) {
	body := fmt.Sprintf(`{"username":%q,"display_name":%q}`, username, username)
	req, err := http.NewRequest("POST", mmURL+"/api/v4/bots", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create bot returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var bot mmBot
	if err := json.NewDecoder(resp.Body).Decode(&bot); err != nil {
		return nil, err
	}
	return &bot, nil
}

func (r *AgentReconciler) createBotToken(mmURL, adminToken, botUserID string) (*mmToken, error) {
	body := `{"description":"operator-managed token"}`
	req, err := http.NewRequest("POST", mmURL+"/api/v4/users/"+botUserID+"/tokens", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create token returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var token mmToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *AgentReconciler) checkAgentServiceAccount(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	desired := mattermostApp.GenerateAgentServiceAccount(agent)

	err := r.Resources.CreateServiceAccountIfNotExists(agent, desired, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to create agent service account")
	}

	current := &corev1.ServiceAccount{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return errors.Wrap(err, "failed to get agent service account")
	}

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *AgentReconciler) checkAgentService(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	desired := mattermostApp.GenerateAgentService(agent)

	err := r.Resources.CreateServiceIfNotExists(agent, desired, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to create agent service")
	}

	current := &corev1.Service{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return errors.Wrap(err, "failed to get agent service")
	}

	resources.CopyServiceEmptyAutoAssignedFields(desired, current)

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *AgentReconciler) checkAgentDeployment(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	desired := mattermostApp.GenerateAgentDeployment(agent)

	err := r.Resources.CreateDeploymentIfNotExists(agent, desired, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to create agent deployment")
	}

	current := &appsv1.Deployment{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return errors.Wrap(err, "failed to get agent deployment")
	}

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *AgentReconciler) checkAgentNetworkPolicy(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	desired := mattermostApp.GenerateAgentNetworkPolicy(agent)

	err := r.Resources.CreateNetworkPolicyIfNotExists(agent, desired, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to create agent network policy")
	}

	current := &networkingv1.NetworkPolicy{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return errors.Wrap(err, "failed to get agent network policy")
	}

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *AgentReconciler) checkAgentHealth(ctx context.Context, agent *mmv1beta.Agent, currentStatus mmv1beta.AgentStatus, reqLogger logr.Logger) (mmv1beta.AgentStatus, error) {
	status := mmv1beta.AgentStatus{
		State:              mmv1beta.Reconciling,
		ObservedGeneration: agent.Generation,
		Endpoint:           fmt.Sprintf("%s.%s.svc.cluster.local:%d", agent.Name, agent.Namespace, mmv1beta.AgentGRPCPort),
	}

	deployment := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deployment)
	if err != nil {
		return status, errors.Wrap(err, "failed to get agent deployment for health check")
	}

	if deployment.Status.ReadyReplicas < 1 {
		return status, fmt.Errorf("agent deployment has %d ready replicas, need at least 1", deployment.Status.ReadyReplicas)
	}

	status.State = mmv1beta.Stable
	return status, nil
}
