package mattermost

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	mattermostv1beta1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
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

// setStateReconciling sets the Mattermost state to reconciling.
func (r *MattermostReconciler) setStateReconciling(mattermost *mattermostv1beta1.Mattermost, reqLogger logr.Logger) error {
	return r.setState(mattermost, mattermostv1beta1.Reconciling, reqLogger)
}

// setStateReconcilingAndLogError attempts to set the Mattermost state
// to reconciling. Any errors attempting this are logged, but not returned. This
// should only be used when the outcome of setting the state can be ignored.
func (r *MattermostReconciler) setStateReconcilingAndLogError(mattermost *mattermostv1beta1.Mattermost, reqLogger logr.Logger) {
	err := r.setStateReconciling(mattermost, reqLogger)
	if err != nil {
		reqLogger.Error(err, "Failed to set state to reconciling")
	}
}

// setState sets the provided Mattermost to the provided state if that
// is different from the current state.
func (r *MattermostReconciler) setState(mattermost *mattermostv1beta1.Mattermost, desired mattermostv1beta1.RunningState, reqLogger logr.Logger) error {
	if mattermost.Status.State == desired {
		return nil
	}

	status := mattermost.Status
	status.State = desired
	err := r.updateStatus(mattermost, status, reqLogger)
	if err != nil {
		return errors.Wrapf(err, "failed to set state to %s", desired)
	}

	return nil
}

func (r *MattermostReconciler) updateStatus(mattermost *mattermostv1beta1.Mattermost, status mattermostv1beta1.MattermostStatus, reqLogger logr.Logger) error {
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
