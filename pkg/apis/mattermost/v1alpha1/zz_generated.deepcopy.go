// +build !ignore_autogenerated

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

// Code generated by deepcopy-gen. DO NOT EDIT.

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AppDeployment) DeepCopyInto(out *AppDeployment) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AppDeployment.
func (in *AppDeployment) DeepCopy() *AppDeployment {
	if in == nil {
		return nil
	}
	out := new(AppDeployment)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BlueGreen) DeepCopyInto(out *BlueGreen) {
	*out = *in
	out.Blue = in.Blue
	out.Green = in.Green
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BlueGreen.
func (in *BlueGreen) DeepCopy() *BlueGreen {
	if in == nil {
		return nil
	}
	out := new(BlueGreen)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Canary) DeepCopyInto(out *Canary) {
	*out = *in
	if in.IngressAnnotations != nil {
		in, out := &in.IngressAnnotations, &out.IngressAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Canary.
func (in *Canary) DeepCopy() *Canary {
	if in == nil {
		return nil
	}
	out := new(Canary)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterInstallation) DeepCopyInto(out *ClusterInstallation) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterInstallation.
func (in *ClusterInstallation) DeepCopy() *ClusterInstallation {
	if in == nil {
		return nil
	}
	out := new(ClusterInstallation)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterInstallation) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterInstallationList) DeepCopyInto(out *ClusterInstallationList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ClusterInstallation, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterInstallationList.
func (in *ClusterInstallationList) DeepCopy() *ClusterInstallationList {
	if in == nil {
		return nil
	}
	out := new(ClusterInstallationList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterInstallationList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterInstallationSize) DeepCopyInto(out *ClusterInstallationSize) {
	*out = *in
	in.App.DeepCopyInto(&out.App)
	in.Minio.DeepCopyInto(&out.Minio)
	in.Database.DeepCopyInto(&out.Database)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterInstallationSize.
func (in *ClusterInstallationSize) DeepCopy() *ClusterInstallationSize {
	if in == nil {
		return nil
	}
	out := new(ClusterInstallationSize)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterInstallationSpec) DeepCopyInto(out *ClusterInstallationSpec) {
	*out = *in
	in.Resources.DeepCopyInto(&out.Resources)
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Affinity != nil {
		in, out := &in.Affinity, &out.Affinity
		*out = new(v1.Affinity)
		(*in).DeepCopyInto(*out)
	}
	in.Minio.DeepCopyInto(&out.Minio)
	in.Database.DeepCopyInto(&out.Database)
	out.BlueGreen = in.BlueGreen
	in.Canary.DeepCopyInto(&out.Canary)
	out.ElasticSearch = in.ElasticSearch
	if in.ServiceAnnotations != nil {
		in, out := &in.ServiceAnnotations, &out.ServiceAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.IngressAnnotations != nil {
		in, out := &in.IngressAnnotations, &out.IngressAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterInstallationSpec.
func (in *ClusterInstallationSpec) DeepCopy() *ClusterInstallationSpec {
	if in == nil {
		return nil
	}
	out := new(ClusterInstallationSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterInstallationStatus) DeepCopyInto(out *ClusterInstallationStatus) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterInstallationStatus.
func (in *ClusterInstallationStatus) DeepCopy() *ClusterInstallationStatus {
	if in == nil {
		return nil
	}
	out := new(ClusterInstallationStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ComponentSize) DeepCopyInto(out *ComponentSize) {
	*out = *in
	in.Resources.DeepCopyInto(&out.Resources)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ComponentSize.
func (in *ComponentSize) DeepCopy() *ComponentSize {
	if in == nil {
		return nil
	}
	out := new(ComponentSize)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Database) DeepCopyInto(out *Database) {
	*out = *in
	in.Resources.DeepCopyInto(&out.Resources)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Database.
func (in *Database) DeepCopy() *Database {
	if in == nil {
		return nil
	}
	out := new(Database)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ElasticSearch) DeepCopyInto(out *ElasticSearch) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ElasticSearch.
func (in *ElasticSearch) DeepCopy() *ElasticSearch {
	if in == nil {
		return nil
	}
	out := new(ElasticSearch)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Minio) DeepCopyInto(out *Minio) {
	*out = *in
	in.Resources.DeepCopyInto(&out.Resources)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Minio.
func (in *Minio) DeepCopy() *Minio {
	if in == nil {
		return nil
	}
	out := new(Minio)
	in.DeepCopyInto(out)
	return out
}
