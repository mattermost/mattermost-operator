package clusterinstallation

import (
	"context"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	mattermostv1beta1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/mattermost-operator/pkg/database"
	"github.com/mattermost/mattermost-operator/pkg/utils"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ClusterInstallationReconciler) IsConvertible(ci *mattermostv1alpha1.ClusterInstallation) error {
	if ci.Spec.BlueGreen.Enable {
		return errors.New("ClusterInstallation resource with BlueGreen enabled cannot be converted to Mattermost resource. Disable BlueGreen to enable migration.")
	}

	if ci.Spec.Canary.Enable {
		return errors.New("ClusterInstallation resource with Canary enabled cannot be converted to Mattermost resource. Disable Canary to enable migration.")
	}

	return nil
}

func (r *ClusterInstallationReconciler) ConvertToMM(ci *mattermostv1alpha1.ClusterInstallation) (*mattermostv1beta1.Mattermost, error) {
	mattermost := &mattermostv1beta1.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ci.Name,
			Namespace:   ci.Namespace,
			Labels:      ci.Labels,
			Annotations: ci.Annotations,
		},
		Spec: mattermostv1beta1.MattermostSpec{
			Size:                   ci.Spec.Size,
			Image:                  ci.Spec.Image,
			Version:                ci.Spec.Version,
			Replicas:               convertReplicas(ci.Spec.Replicas),
			MattermostEnv:          ci.Spec.MattermostEnv,
			LicenseSecret:          ci.Spec.MattermostLicenseSecret,
			IngressName:            ci.Spec.IngressName,
			IngressAnnotations:     ci.Spec.IngressAnnotations,
			UseIngressTLS:          ci.Spec.UseIngressTLS,
			UseServiceLoadBalancer: ci.Spec.UseServiceLoadBalancer,
			ServiceAnnotations:     ci.Spec.ServiceAnnotations,
			ResourceLabels:         ci.Spec.ResourceLabels,
			ImagePullPolicy:        ci.Spec.ImagePullPolicy,
			FileStore:              convertFileStore(ci),
			ElasticSearch:          convertElasticSearch(ci.Spec.ElasticSearch),
			Scheduling:             convertScheduling(ci.Spec),
			Probes:                 convertProbes(ci.Spec),
		},
	}

	mmDB, err := r.convertDatabase(ci)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert ClusterInstallation.Spec.Database to Mattermost.Spec.Database")
	}

	mattermost.Spec.Database = mmDB

	return mattermost, nil
}

func convertFileStore(ci *mattermostv1alpha1.ClusterInstallation) mattermostv1beta1.FileStore {
	if !ci.Spec.Minio.IsExternal() {
		return convertToOperatorManagedMinio(ci)
	}

	return mattermostv1beta1.FileStore{
		External: &mattermostv1beta1.ExternalFileStore{
			URL:    ci.Spec.Minio.ExternalURL,
			Bucket: ci.Spec.Minio.ExternalBucket,
			Secret: ci.Spec.Minio.Secret,
		},
	}
}

func convertToOperatorManagedMinio(ci *mattermostv1alpha1.ClusterInstallation) mattermostv1beta1.FileStore {
	return mattermostv1beta1.FileStore{
		OperatorManaged: &mattermostv1beta1.OperatorManagedMinio{
			StorageSize: ci.Spec.Minio.StorageSize,
			Replicas:    convertReplicas(ci.Spec.Minio.Replicas),
			Resources:   ci.Spec.Minio.Resources,
		},
	}
}

func (r *ClusterInstallationReconciler) convertDatabase(ci *mattermostv1alpha1.ClusterInstallation) (mattermostv1beta1.Database, error) {
	if ci.Spec.Database.Secret == "" {
		return convertToOperatorManagedDB(ci), nil
	}

	secret := &corev1.Secret{}
	name := types.NamespacedName{Name: ci.Spec.Database.Secret, Namespace: ci.Namespace}

	err := r.Client.Get(context.TODO(), name, secret)
	if err != nil {
		return mattermostv1beta1.Database{}, errors.Wrap(err, "failed to get Database Secret")
	}

	dbInfo := database.GenerateDatabaseInfoFromSecret(secret)
	if err := dbInfo.IsValid(); err != nil {
		return mattermostv1beta1.Database{}, errors.Wrap(err, "dbInfo generated from Secret is invalid")
	}

	if !dbInfo.IsExternal() {
		return convertToOperatorManagedDB(ci), nil
	}

	return mattermostv1beta1.Database{
		External: &mattermostv1beta1.ExternalDatabase{
			Secret: ci.Spec.Database.Secret,
		},
	}, nil
}

func convertToOperatorManagedDB(ci *mattermostv1alpha1.ClusterInstallation) mattermostv1beta1.Database {
	return mattermostv1beta1.Database{
		OperatorManaged: &mattermostv1beta1.OperatorManagedDatabase{
			Type:                     ci.Spec.Database.Type,
			StorageSize:              ci.Spec.Database.StorageSize,
			Replicas:                 convertReplicas(ci.Spec.Database.Replicas),
			Resources:                ci.Spec.Database.Resources,
			InitBucketURL:            ci.Spec.Database.InitBucketURL,
			BackupSchedule:           ci.Spec.Database.BackupSchedule,
			BackupURL:                ci.Spec.Database.BackupURL,
			BackupRemoteDeletePolicy: ci.Spec.Database.BackupRemoteDeletePolicy,
			BackupSecretName:         ci.Spec.Database.BackupSecretName,
			BackupRestoreSecretName:  ci.Spec.Database.BackupRestoreSecretName,
		},
	}
}

func convertElasticSearch(es mattermostv1alpha1.ElasticSearch) mattermostv1beta1.ElasticSearch {
	return mattermostv1beta1.ElasticSearch{
		Host:     es.Host,
		UserName: es.UserName,
		Password: es.Password,
	}
}

func convertScheduling(spec mattermostv1alpha1.ClusterInstallationSpec) mattermostv1beta1.Scheduling {
	return mattermostv1beta1.Scheduling{
		Resources:    spec.Resources,
		NodeSelector: spec.NodeSelector,
		Affinity:     spec.Affinity,
	}
}

func convertProbes(spec mattermostv1alpha1.ClusterInstallationSpec) mattermostv1beta1.Probes {
	return mattermostv1beta1.Probes{
		LivenessProbe:  spec.LivenessProbe,
		ReadinessProbe: spec.ReadinessProbe,
	}
}

func convertReplicas(old int32) *int32 {
	if old < 0 {
		return utils.NewInt32(0)
	}
	// 0 replicas means non specified, therefore set to nil so that it is set to default
	if old == 0 {
		return nil
	}
	return utils.NewInt32(old)
}
