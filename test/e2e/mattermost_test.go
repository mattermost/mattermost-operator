package e2e

import (
	goctx "context"
	"fmt"
	"testing"
	"time"

	apis "github.com/mattermost/mattermost-operator/pkg/apis"
	operator "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	mysqlOperator "github.com/oracle/mysql-operator/pkg/apis/mysql/v1alpha1"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	retryInterval        = time.Second * 5
	timeout              = time.Second * 300
	cleanupRetryInterval = time.Second * 1
	cleanupTimeout       = time.Second * 5
)

func TestMattermost(t *testing.T) {

	mysqlList := &mysqlOperator.ClusterList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: "mysql.oracle.com/v1alpha1",
		},
	}
	err := framework.AddToFrameworkScheme(mysqlOperator.AddToScheme, mysqlList)
	if err != nil {
		t.Fatalf("failed to add mysql custom resource scheme to framework: %v", err)
	}

	mattermostList := &operator.ClusterInstallationList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterInstallation",
			APIVersion: "mattermost.com/v1alpha1",
		},
	}
	err = framework.AddToFrameworkScheme(apis.AddToScheme, mattermostList)
	if err != nil {
		t.Fatalf("failed to add mattermost custom resource scheme to framework: %v", err)
	}

	// run subtests
	t.Run("mattermost-group", func(t *testing.T) {
		t.Run("Cluster", MattermostCluster)
	})
}

func mattermostScaleTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return fmt.Errorf("could not get namespace: %v", err)
	}
	// create memcached custom resource
	exampleMattermost := &operator.ClusterInstallation{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterInstallation",
			APIVersion: "mattermost.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mm",
			Namespace: namespace,
		},
		Spec: operator.ClusterInstallationSpec{
			IngressName: "test-example.mattermost.dev",
			Replicas:    1,
		},
	}

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err = f.Client.Create(goctx.TODO(), exampleMattermost, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		return err
	}
	// wait for test-mm to reach 2 replicas
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "test-mm", 1, retryInterval, timeout)
	if err != nil {
		return err
	}

	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "test-mm", Namespace: namespace}, exampleMattermost)
	if err != nil {
		return err
	}
	// exampleMattermost.Spec.Replicas = 3
	// err = f.Client.Update(goctx.TODO(), exampleMattermost)
	// if err != nil {
	// 	return err
	// }

	// wait for test-mm to reach 3 replicas
	// return e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "test-mm", 3, retryInterval, timeout)
	return nil
}

func MattermostCluster(t *testing.T) {
	ctx := framework.NewTestCtx(t)
	defer ctx.Cleanup()

	err := ctx.InitializeClusterResources(&framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		t.Fatalf("failed to initialize cluster resources: %v", err)
	}
	t.Log("Initialized cluster resources")
	namespace, err := ctx.GetNamespace()
	if err != nil {
		t.Fatal(err)
	}
	// get global framework variables
	f := framework.Global

	// wait for mysql-operator to be ready
	err = e2eutil.WaitForDeployment(t, f.KubeClient, "mysql-operator", "mysql-operator", 1, retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}

	// wait for mattermost-operator to be ready
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "mattermost-operator", 1, retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}

	if err = mattermostScaleTest(t, f, ctx); err != nil {
		t.Fatal(err)
	}
}
