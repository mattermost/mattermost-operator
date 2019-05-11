package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	mattermostES "github.com/mattermost/mattermost-operator/pkg/components/es"

	esOperator "github.com/upmc-enterprises/elasticsearch-operator/pkg/apis/elasticsearchoperator/v1"
)

func (r *ReconcileClusterInstallation) createESDeploymentIfNotExists(mattermost *mattermostv1alpha1.ClusterInstallation, deployment *esOperator.ElasticsearchCluster, reqLogger logr.Logger) error {
	foundDeployment := &esOperator.ElasticsearchCluster{}
	errGet := r.client.Get(context.TODO(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, foundDeployment)
	if errGet != nil && errors.IsNotFound(errGet) {
		return r.createResource(mattermost, deployment, reqLogger)
	} else if errGet != nil {
		reqLogger.Error(errGet, "ClusterInstallation ElasticSearch")
		return errGet
	}

	if !reflect.DeepEqual(foundDeployment.Spec, deployment.Spec) {
		foundDeployment.Spec = deployment.Spec
		reqLogger.Info("Updating ElasticSearch deployment", deployment.Namespace, deployment.Name)
		err := r.client.Update(context.TODO(), foundDeployment)
		if err != nil {
			return err
		}
		_ = controllerutil.SetControllerReference(mattermost, foundDeployment, r.scheme)
	}

	return nil
}

func (r *ReconcileClusterInstallation) checkESDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	return r.createESDeploymentIfNotExists(mattermost, mattermostES.ESInstance(mattermost), reqLogger)
}

func (r *ReconcileClusterInstallation) getESService(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) (string, error) {
	esServiceName := fmt.Sprintf("%s-es", mattermost.Name)
	esService := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: esServiceName, Namespace: mattermost.Namespace}, esService)
	if err != nil {
		return "", err
	}

	connectionString := fmt.Sprintf("%s.%s.svc.cluster.local:%d", esService.Name, mattermost.Namespace, esService.Spec.Ports[0].Port)
	return connectionString, nil
}
