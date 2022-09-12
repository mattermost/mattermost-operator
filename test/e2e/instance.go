package e2e

import (
	"context"
	"testing"
	"time"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	mattermostInstanceCreationTimeout = 15 * time.Second
	mattermostInstanceWaitTimeout     = 3 * time.Minute
	mattermostInstanceGetTimeout      = 60 * time.Second
	mattermostInstanceUpdateTimeout   = 60 * time.Second
)

// mattermostInstance defines a mattermost instance that is created in the test k8s cluster in order to ease the creation,
// destruction and testing of the defined spec.
// TODO: Ensure creation and deletion only happen once using sync.Once ? Not sure this is necessary since this is a testing
// tool.
type mattermostInstance struct {
	t              *testing.T
	mattermostSpec *mmv1beta.Mattermost
	timeoutCreate  time.Duration
	timeoutWait    time.Duration
	timeoutGet     time.Duration
	timeoutUpdate  time.Duration
	k8sClient      client.Client
	namespaceName  types.NamespacedName
	created        bool
}

// Namespace returns the NamespacedName in order to retrieve the object easily
func (m *mattermostInstance) Namespace() types.NamespacedName {
	return m.namespaceName
}

// Create creates the instance within the cluster
func (m *mattermostInstance) Create() {
	m.namespaceName = types.NamespacedName{
		Namespace: m.mattermostSpec.Namespace,
		Name:      m.mattermostSpec.Name,
	}

	ctx, cancel := context.WithTimeout(context.Background(), m.timeoutCreate)
	defer cancel()

	err := m.k8sClient.Create(ctx, m.mattermostSpec)
	require.NoError(m.t, err)

	m.created = true
}

// CreateAndWait Creates the instance within the cluster and waits until is a stable instance (failing if not)
func (m *mattermostInstance) CreateAndWait() {
	m.Create()
	m.Wait()
}

// Wait waits for the mattermost instance to be stable
func (m *mattermostInstance) Wait() {
	err := WaitForMattermostStable(m.t, m.k8sClient, m.Namespace(), m.timeoutWait)
	require.NoError(m.t, err, "Timed out waiting for a mattermost instance to become stable")
}

// Destroy destroys the created instance
func (m *mattermostInstance) Destroy() {
	if !m.created {
		return
	}

	err := m.k8sClient.Delete(context.Background(), m.mattermostSpec)
	require.NoError(m.t, err)
}

// Get retrieves the mattermost instance spec from the cluster
func (m *mattermostInstance) Get() mmv1beta.Mattermost {
	ctx, cancel := context.WithTimeout(context.Background(), m.timeoutGet)
	defer cancel()

	var mattermost mmv1beta.Mattermost
	err := m.k8sClient.Get(ctx, m.Namespace(), &mattermost)
	require.NoError(m.t, err, "Error retrieving created mattermost instance from cluster")

	return mattermost
}

// Update updates the mattermost instance definition
func (m *mattermostInstance) Update(mattermost *mmv1beta.Mattermost) {
	ctx, cancel := context.WithTimeout(context.Background(), m.timeoutUpdate)
	defer cancel()
	err := m.k8sClient.Update(ctx, mattermost)
	require.NoError(m.t, err, "Error updating mattermost instance")
}

// UpdateAndWait Updates the mattermost instance definition and waits for the instance to be stable
func (m *mattermostInstance) UpdateAndWait(mattermost *mmv1beta.Mattermost) {
	m.Update(mattermost)
	m.Wait()
}

func NewMattermostInstance(t *testing.T, k8sClient client.Client, mattermost *mmv1beta.Mattermost) *mattermostInstance {
	return &mattermostInstance{
		t:              t,
		k8sClient:      k8sClient,
		mattermostSpec: mattermost,
		timeoutCreate:  mattermostInstanceCreationTimeout,
		timeoutWait:    mattermostInstanceWaitTimeout,
		timeoutGet:     mattermostInstanceGetTimeout,
		timeoutUpdate:  mattermostInstanceUpdateTimeout,
	}
}

func ExampleMattermostInstance() {
	var t testing.T
	var k8sClient client.Client
	specName := "mm-provided-name"

	mattermost := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name: specName,
		},
		// ...
	}

	// Setup the test instance
	instance := NewMattermostInstance(&t, k8sClient, mattermost)
	defer instance.Destroy()

	// Create the instance on the cluster and wait for creation
	instance.CreateAndWait()

	// Retrieve the instance to check against it
	clusterMattermost := instance.Get()

	// Tests here
	if clusterMattermost.Name != specName {
		t.Errorf("Name should be `%s`", specName)
	}
}
