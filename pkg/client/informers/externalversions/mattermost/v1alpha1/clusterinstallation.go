/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	time "time"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	versioned "github.com/mattermost/mattermost-operator/pkg/client/clientset/versioned"
	internalinterfaces "github.com/mattermost/mattermost-operator/pkg/client/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/mattermost/mattermost-operator/pkg/client/listers/mattermost/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// ClusterInstallationInformer provides access to a shared informer and lister for
// ClusterInstallations.
type ClusterInstallationInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.ClusterInstallationLister
}

type clusterInstallationInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewClusterInstallationInformer constructs a new informer for ClusterInstallation type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewClusterInstallationInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredClusterInstallationInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredClusterInstallationInformer constructs a new informer for ClusterInstallation type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredClusterInstallationInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.MattermostV1alpha1().ClusterInstallations(namespace).List(options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.MattermostV1alpha1().ClusterInstallations(namespace).Watch(options)
			},
		},
		&mattermostv1alpha1.ClusterInstallation{},
		resyncPeriod,
		indexers,
	)
}

func (f *clusterInstallationInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredClusterInstallationInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *clusterInstallationInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&mattermostv1alpha1.ClusterInstallation{}, f.defaultInformer)
}

func (f *clusterInstallationInformer) Lister() v1alpha1.ClusterInstallationLister {
	return v1alpha1.NewClusterInstallationLister(f.Informer().GetIndexer())
}
