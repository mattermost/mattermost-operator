// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package mattermostrestoredb

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
)

func (r *MattermostRestoreDBReconciler) updateStatus(mattermost *mattermostv1alpha1.MattermostRestoreDB, status mattermostv1alpha1.MattermostRestoreDBStatus, reqLogger logr.Logger) error {
	if !reflect.DeepEqual(mattermost.Status, status) {
		mattermost.Status = status
		err := r.Client.Status().Update(context.TODO(), mattermost)
		if err != nil {
			reqLogger.Error(err, "failed to update the mattermostrestoredb status")
			return err
		}
	}

	return nil
}
