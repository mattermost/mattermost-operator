package mattermost

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *MattermostReconciler) assertSecretContains(secretName, keyName, namespace string) error {
	foundSecret := &corev1.Secret{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, foundSecret)
	if err != nil {
		return errors.Wrap(err, "error getting secret")
	}

	for key := range foundSecret.Data {
		if keyName == key {
			return nil
		}
	}

	return fmt.Errorf("secret %s is missing data key: %s", secretName, keyName)
}

// updateStatusReconciling sets the Mattermost state to reconciling.
func (r *MattermostReconciler) updateStatusReconciling(mattermost *mmv1beta.Mattermost, status mmv1beta.MattermostStatus, reqLogger logr.Logger) error {
	status.State = mmv1beta.Reconciling
	return r.updateStatus(mattermost, status, reqLogger)
}

// updateStatusReconcilingAndLogError attempts to set the Mattermost state to reconciling
// and updates the status. Any errors attempting this are logged, but not returned.
// This should only be used when the outcome of setting the state can be ignored.
func (r *MattermostReconciler) updateStatusReconcilingAndLogError(mattermost *mmv1beta.Mattermost, status mmv1beta.MattermostStatus, reqLogger logr.Logger, statusErr error) {
	if statusErr != nil {
		status.Error = statusErr.Error()
	}
	err := r.updateStatusReconciling(mattermost, status, reqLogger)
	if err != nil {
		reqLogger.Error(err, "Failed to set state to reconciling")
	}
}

func (r *MattermostReconciler) updateStatus(mattermost *mmv1beta.Mattermost, status mmv1beta.MattermostStatus, reqLogger logr.Logger) error {
	if reflect.DeepEqual(mattermost.Status, status) {
		return nil
	}

	if mattermost.Status.State != status.State {
		reqLogger.Info(fmt.Sprintf("Updating Mattermost state from '%s' to '%s'", mattermost.Status.State, status.State))
	}

	mattermost.Status = status
	err := r.Client.Status().Update(context.TODO(), mattermost)
	if err != nil {
		return errors.Wrap(err, "failed to update the Mattermost status")
	}

	return nil
}
