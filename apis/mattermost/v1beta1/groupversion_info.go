// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

// Package v1beta1 contains API Schema definitions for the mattermost v1beta1 API group
// +kubebuilder:object:generate=true
// +groupName=installation.mattermost.com
package v1beta1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: "installation.mattermost.com", Version: "v1beta1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme

	// Generated clients need a `GroupVersion` with name `SchemeGroupVersion`
	SchemeGroupVersion = GroupVersion
)

// Resource takes an unqualified resource and returns a Group qualified GroupResource
// Function is not generated but required by generated clients
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}
