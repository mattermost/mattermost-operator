package es

import (
	"fmt"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"

	esOperator "github.com/upmc-enterprises/elasticsearch-operator/pkg/apis/elasticsearchoperator/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ESInstance returns the ES component to deploy
func ESInstance(mattermost *mattermostv1alpha1.ClusterInstallation) *esOperator.ElasticsearchCluster {
	esName := fmt.Sprintf("%s-es", mattermost.Name)

	esInstance := &esOperator.ElasticsearchCluster{}
	esInstance.SetName(esName)
	esInstance.SetNamespace(mattermost.Namespace)

	ownerRef := []metav1.OwnerReference{
		*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
			Group:   mattermostv1alpha1.SchemeGroupVersion.Group,
			Version: mattermostv1alpha1.SchemeGroupVersion.Version,
			Kind:    "ClusterInstallation",
		}),
	}
	esInstance.SetOwnerReferences(ownerRef)

	// Spec Section
	esInstance.Spec.ElasticSearchImage = "upmcenterprises/docker-elasticsearch-kubernetes:6.1.3_0"
	esInstance.Spec.NetworkHost = "0.0.0.0"
	esInstance.Spec.JavaOptions = "-Xms1024m -Xmx1024m"
	esInstance.Spec.ClientNodeReplicas = mattermost.Spec.ElasticSearchOptions.ClientNodeReplicas
	esInstance.Spec.MasterNodeReplicas = mattermost.Spec.ElasticSearchOptions.MasterNodeReplicas
	esInstance.Spec.DataNodeReplicas = mattermost.Spec.ElasticSearchOptions.DataNodeReplicas
	esInstance.Spec.DataDiskSize = mattermost.Spec.ElasticSearchOptions.EsStorageSize
	esInstance.Spec.Resources.Limits.CPU = mattermost.Spec.ElasticSearchOptions.Resources.Limits.CPU
	esInstance.Spec.Resources.Limits.Memory = mattermost.Spec.ElasticSearchOptions.Resources.Limits.Memory
	esInstance.Spec.Resources.Requests.CPU = mattermost.Spec.ElasticSearchOptions.Resources.Requests.CPU
	esInstance.Spec.Resources.Requests.Memory = mattermost.Spec.ElasticSearchOptions.Resources.Requests.Memory

	return esInstance
}
