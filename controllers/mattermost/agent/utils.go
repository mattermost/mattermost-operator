package agent

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/pkg/errors"
)

func (r *AgentReconciler) updateStatusReconciling(ctx context.Context, agent *mmv1beta.Agent, status mmv1beta.AgentStatus, reqLogger logr.Logger) error {
	status.State = mmv1beta.Reconciling
	return r.updateStatus(ctx, agent, status, reqLogger)
}

func (r *AgentReconciler) updateStatusReconcilingAndLogError(ctx context.Context, agent *mmv1beta.Agent, status mmv1beta.AgentStatus, reqLogger logr.Logger, statusErr error) {
	if statusErr != nil {
		status.Error = statusErr.Error()
	}
	err := r.updateStatusReconciling(ctx, agent, status, reqLogger)
	if err != nil {
		reqLogger.Error(err, "Failed to set agent state to reconciling")
	}
}

func (r *AgentReconciler) updateStatus(ctx context.Context, agent *mmv1beta.Agent, status mmv1beta.AgentStatus, reqLogger logr.Logger) error {
	if reflect.DeepEqual(agent.Status, status) {
		return nil
	}

	if agent.Status.State != status.State {
		reqLogger.Info(fmt.Sprintf("Updating Agent state from '%s' to '%s'", agent.Status.State, status.State))
	}

	agent.Status = status
	err := r.Client.Status().Update(ctx, agent)
	if err != nil {
		return errors.Wrap(err, "failed to update the Agent status")
	}

	return nil
}
