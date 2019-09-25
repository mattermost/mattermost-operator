// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package mattermostrestoredb

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
)

func (r *ReconcileMattermostRestoreDB) updateMySQLSecrets(mattermostRestore *mattermostv1alpha1.MattermostRestoreDB, reqLogger logr.Logger) error {
	dbSecretName := fmt.Sprintf("%s-mysql-root-password", mattermostRestore.Spec.MattermostClusterName)
	dbSecret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: dbSecretName, Namespace: mattermostRestore.Namespace}, dbSecret)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Error(err, "MySQL secret does not exist")
		return err
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if mysql secret exists")
		return err
	}

	userName := "mmuser"
	if mattermostRestore.Spec.MattermostDBUser != "" {
		userName = mattermostRestore.Spec.MattermostDBUser
	}

	userPassword := string(dbSecret.Data["PASSWORD"])
	if mattermostRestore.Spec.MattermostDBPassword != "" {
		userPassword = mattermostRestore.Spec.MattermostDBPassword
	}

	databaseName := "mattermost"
	if mattermostRestore.Spec.MattermostDBName != "" {
		databaseName = mattermostRestore.Spec.MattermostDBName
	}

	dbSecret.StringData = map[string]string{
		"USER":     userName,
		"DATABASE": databaseName,
		"PASSWORD": userPassword,
	}

	err = r.client.Update(context.TODO(), dbSecret)
	if err != nil {
		reqLogger.Error(err, "Failed to update the mysql secret")
		return err
	}

	return nil
}
