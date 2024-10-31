//go:build !ignore_autogenerated

// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AppDeployment) DeepCopyInto(out *AppDeployment) {
	*out = *in
	if in.ResourceLabels != nil {
		in, out := &in.ResourceLabels, &out.ResourceLabels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
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
	in.Blue.DeepCopyInto(&out.Blue)
	in.Green.DeepCopyInto(&out.Green)
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
	in.Deployment.DeepCopyInto(&out.Deployment)
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
	in.Status.DeepCopyInto(&out.Status)
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
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ClusterInstallation, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
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
	in.BlueGreen.DeepCopyInto(&out.BlueGreen)
	in.Canary.DeepCopyInto(&out.Canary)
	out.ElasticSearch = in.ElasticSearch
	if in.ServiceAnnotations != nil {
		in, out := &in.ServiceAnnotations, &out.ServiceAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.ResourceLabels != nil {
		in, out := &in.ResourceLabels, &out.ResourceLabels
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
	if in.MattermostEnv != nil {
		in, out := &in.MattermostEnv, &out.MattermostEnv
		*out = make([]v1.EnvVar, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.LivenessProbe.DeepCopyInto(&out.LivenessProbe)
	in.ReadinessProbe.DeepCopyInto(&out.ReadinessProbe)
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
	if in.Migration != nil {
		in, out := &in.Migration, &out.Migration
		*out = new(MigrationStatus)
		**out = **in
	}
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
func (in *MattermostRestoreDB) DeepCopyInto(out *MattermostRestoreDB) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MattermostRestoreDB.
func (in *MattermostRestoreDB) DeepCopy() *MattermostRestoreDB {
	if in == nil {
		return nil
	}
	out := new(MattermostRestoreDB)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *MattermostRestoreDB) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MattermostRestoreDBList) DeepCopyInto(out *MattermostRestoreDBList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]MattermostRestoreDB, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MattermostRestoreDBList.
func (in *MattermostRestoreDBList) DeepCopy() *MattermostRestoreDBList {
	if in == nil {
		return nil
	}
	out := new(MattermostRestoreDBList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *MattermostRestoreDBList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MattermostRestoreDBSpec) DeepCopyInto(out *MattermostRestoreDBSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MattermostRestoreDBSpec.
func (in *MattermostRestoreDBSpec) DeepCopy() *MattermostRestoreDBSpec {
	if in == nil {
		return nil
	}
	out := new(MattermostRestoreDBSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MattermostRestoreDBStatus) DeepCopyInto(out *MattermostRestoreDBStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MattermostRestoreDBStatus.
func (in *MattermostRestoreDBStatus) DeepCopy() *MattermostRestoreDBStatus {
	if in == nil {
		return nil
	}
	out := new(MattermostRestoreDBStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MigrationStatus) DeepCopyInto(out *MigrationStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MigrationStatus.
func (in *MigrationStatus) DeepCopy() *MigrationStatus {
	if in == nil {
		return nil
	}
	out := new(MigrationStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Minio) DeepCopyInto(out *Minio) {
	*out = *in
	in.Resources.DeepCopyInto(&out.Resources)
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
