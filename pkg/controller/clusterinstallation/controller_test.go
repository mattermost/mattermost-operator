package clusterinstallation

import (
	"testing"
	"time"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	minioOperator "github.com/minio/minio-operator/pkg/apis/miniocontroller/v1beta1"
	"github.com/onsi/gomega"
	mysqlOperator "github.com/oracle/mysql-operator/pkg/apis/mysql/v1alpha1"
	esOperator "github.com/upmc-enterprises/elasticsearch-operator/pkg/apis/elasticsearchoperator/v1"

	"golang.org/x/net/context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	rbacv1beta1 "k8s.io/api/rbac/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}
var depKey = types.NamespacedName{Name: "foo", Namespace: "default"}
var depIngressKey = types.NamespacedName{Name: "foo-ingress", Namespace: "default"}
var depMysqlKey = types.NamespacedName{Name: "foo-mysql", Namespace: "default"}
var depMinioKey = types.NamespacedName{Name: "foo-minio", Namespace: "default"}
var depESKey = types.NamespacedName{Name: "foo-es", Namespace: "default"}
var depSvcAccountKey = types.NamespacedName{Name: "mysql-agent", Namespace: "default"}

const timeout = time.Second * 60

func TestReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	instance := &mattermostv1alpha1.ClusterInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: mattermostv1alpha1.ClusterInstallationSpec{
			IngressName:         "foo.mattermost.dev",
			EnableElasticSearch: true,
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())
	defer close(StartTestManager(mgr, g))

	// Create the ClusterInstallation object and expect the Reconcile and Deployment to be created
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	// Mysql test section
	mysql := &mysqlOperator.Cluster{}
	g.Eventually(func() error { return c.Get(context.TODO(), depMysqlKey, mysql) }, timeout).
		Should(gomega.Succeed())

	svcAccount := &corev1.ServiceAccount{}
	g.Eventually(func() error { return c.Get(context.TODO(), depSvcAccountKey, svcAccount) }, timeout).
		Should(gomega.Succeed())

	roleBinding := &rbacv1beta1.RoleBinding{}
	g.Eventually(func() error { return c.Get(context.TODO(), depSvcAccountKey, roleBinding) }, timeout).
		Should(gomega.Succeed())

	// Minio test section
	minio := &minioOperator.MinioInstance{}
	g.Eventually(func() error { return c.Get(context.TODO(), depMinioKey, minio) }, timeout).
		Should(gomega.Succeed())

	// ES test section
	es := &esOperator.ElasticsearchCluster{}
	g.Eventually(func() error { return c.Get(context.TODO(), depESKey, es) }, timeout).
		Should(gomega.Succeed())

	// Mattermost test section
	service := &corev1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), depKey, service) }, timeout).
		Should(gomega.Succeed())

	ingress := &v1beta1.Ingress{}
	g.Eventually(func() error { return c.Get(context.TODO(), depIngressKey, ingress) }, timeout).
		Should(gomega.Succeed())

	// Create the mysql secret
	dbSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-mysql-root-password",
			Namespace: instance.Namespace,
		},
		Data: map[string][]byte{
			"password": []byte("mysupersecure"),
		},
	}
	err = c.Create(context.TODO(), dbSecret)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	defer c.Delete(context.TODO(), dbSecret)

	// Create the minio secret
	MinioSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-minio",
			Namespace: instance.Namespace,
		},
		Data: map[string][]byte{
			"accesskey": []byte("mysupersecure"),
			"secretkey": []byte("mysupersecurekey"),
		},
	}
	err = c.Create(context.TODO(), dbSecret)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	defer c.Delete(context.TODO(), MinioSecret)

	// Create the minio service
	minioPort := corev1.ServicePort{Port: 9000}
	minioService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-minio",
			Namespace: instance.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports:     []corev1.ServicePort{minioPort},
			ClusterIP: corev1.ClusterIPNone,
		},
	}
	err = c.Create(context.TODO(), minioService)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	defer c.Delete(context.TODO(), minioService)

	// Create the es service
	esPort := corev1.ServicePort{Port: 9200}
	esService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "elasticsearch-" + instance.Name + "-es",
			Namespace: instance.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports:     []corev1.ServicePort{esPort},
			ClusterIP: corev1.ClusterIPNone,
		},
	}
	err = c.Create(context.TODO(), esService)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	defer c.Delete(context.TODO(), esService)

	deploy := &appsv1.Deployment{}
	g.Eventually(func() error { return c.Get(context.TODO(), depKey, deploy) }, timeout).
		Should(gomega.Succeed())

	// Delete the Deployment and expect Reconcile to be called for Deployment deletion
	g.Expect(c.Delete(context.TODO(), deploy)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(func() error { return c.Get(context.TODO(), depKey, deploy) }, timeout).
		Should(gomega.Succeed())

	g.Expect(c.Delete(context.TODO(), ingress)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(func() error { return c.Get(context.TODO(), depIngressKey, ingress) }, timeout).
		Should(gomega.Succeed())

	g.Expect(c.Delete(context.TODO(), service)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(func() error { return c.Get(context.TODO(), depKey, service) }, timeout).
		Should(gomega.Succeed())

}
