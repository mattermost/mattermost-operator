// +build !ignore_autogenerated

// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

// Code generated by controller-gen. DO NOT EDIT.

package v1beta1

import (
	"k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Database) DeepCopyInto(out *Database) {
	*out = *in
	if in.External != nil {
		in, out := &in.External, &out.External
		*out = new(ExternalDatabase)
		**out = **in
	}
	if in.OperatorManaged != nil {
		in, out := &in.OperatorManaged, &out.OperatorManaged
		*out = new(OperatorManagedDatabase)
		(*in).DeepCopyInto(*out)
	}
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
func (in *ExternalDatabase) DeepCopyInto(out *ExternalDatabase) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ExternalDatabase.
func (in *ExternalDatabase) DeepCopy() *ExternalDatabase {
	if in == nil {
		return nil
	}
	out := new(ExternalDatabase)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ExternalFileStore) DeepCopyInto(out *ExternalFileStore) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ExternalFileStore.
func (in *ExternalFileStore) DeepCopy() *ExternalFileStore {
	if in == nil {
		return nil
	}
	out := new(ExternalFileStore)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FileStore) DeepCopyInto(out *FileStore) {
	*out = *in
	if in.External != nil {
		in, out := &in.External, &out.External
		*out = new(ExternalFileStore)
		**out = **in
	}
	if in.OperatorManaged != nil {
		in, out := &in.OperatorManaged, &out.OperatorManaged
		*out = new(OperatorManagedMinio)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FileStore.
func (in *FileStore) DeepCopy() *FileStore {
	if in == nil {
		return nil
	}
	out := new(FileStore)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Ingress) DeepCopyInto(out *Ingress) {
	*out = *in
	if in.Annotations != nil {
		in, out := &in.Annotations, &out.Annotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.IngressClass != nil {
		in, out := &in.IngressClass, &out.IngressClass
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Ingress.
func (in *Ingress) DeepCopy() *Ingress {
	if in == nil {
		return nil
	}
	out := new(Ingress)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Mattermost) DeepCopyInto(out *Mattermost) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Mattermost.
func (in *Mattermost) DeepCopy() *Mattermost {
	if in == nil {
		return nil
	}
	out := new(Mattermost)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Mattermost) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MattermostList) DeepCopyInto(out *MattermostList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Mattermost, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MattermostList.
func (in *MattermostList) DeepCopy() *MattermostList {
	if in == nil {
		return nil
	}
	out := new(MattermostList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *MattermostList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MattermostSpec) DeepCopyInto(out *MattermostSpec) {
	*out = *in
	if in.Replicas != nil {
		in, out := &in.Replicas, &out.Replicas
		*out = new(int32)
		**out = **in
	}
	if in.MattermostEnv != nil {
		in, out := &in.MattermostEnv, &out.MattermostEnv
		*out = make([]v1.EnvVar, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.IngressAnnotations != nil {
		in, out := &in.IngressAnnotations, &out.IngressAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
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
	if in.Ingress != nil {
		in, out := &in.Ingress, &out.Ingress
		*out = new(Ingress)
		(*in).DeepCopyInto(*out)
	}
	if in.Volumes != nil {
		in, out := &in.Volumes, &out.Volumes
		*out = make([]v1.Volume, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.VolumeMounts != nil {
		in, out := &in.VolumeMounts, &out.VolumeMounts
		*out = make([]v1.VolumeMount, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.ImagePullSecrets != nil {
		in, out := &in.ImagePullSecrets, &out.ImagePullSecrets
		*out = make([]v1.LocalObjectReference, len(*in))
		copy(*out, *in)
	}
	in.Database.DeepCopyInto(&out.Database)
	in.FileStore.DeepCopyInto(&out.FileStore)
	out.ElasticSearch = in.ElasticSearch
	in.Scheduling.DeepCopyInto(&out.Scheduling)
	in.Probes.DeepCopyInto(&out.Probes)
	in.PodExtensions.DeepCopyInto(&out.PodExtensions)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MattermostSpec.
func (in *MattermostSpec) DeepCopy() *MattermostSpec {
	if in == nil {
		return nil
	}
	out := new(MattermostSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MattermostStatus) DeepCopyInto(out *MattermostStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MattermostStatus.
func (in *MattermostStatus) DeepCopy() *MattermostStatus {
	if in == nil {
		return nil
	}
	out := new(MattermostStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OperatorManagedDatabase) DeepCopyInto(out *OperatorManagedDatabase) {
	*out = *in
	if in.Replicas != nil {
		in, out := &in.Replicas, &out.Replicas
		*out = new(int32)
		**out = **in
	}
	in.Resources.DeepCopyInto(&out.Resources)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OperatorManagedDatabase.
func (in *OperatorManagedDatabase) DeepCopy() *OperatorManagedDatabase {
	if in == nil {
		return nil
	}
	out := new(OperatorManagedDatabase)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OperatorManagedMinio) DeepCopyInto(out *OperatorManagedMinio) {
	*out = *in
	if in.Replicas != nil {
		in, out := &in.Replicas, &out.Replicas
		*out = new(int32)
		**out = **in
	}
	in.Resources.DeepCopyInto(&out.Resources)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OperatorManagedMinio.
func (in *OperatorManagedMinio) DeepCopy() *OperatorManagedMinio {
	if in == nil {
		return nil
	}
	out := new(OperatorManagedMinio)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodExtensions) DeepCopyInto(out *PodExtensions) {
	*out = *in
	if in.InitContainers != nil {
		in, out := &in.InitContainers, &out.InitContainers
		*out = make([]v1.Container, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodExtensions.
func (in *PodExtensions) DeepCopy() *PodExtensions {
	if in == nil {
		return nil
	}
	out := new(PodExtensions)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Probes) DeepCopyInto(out *Probes) {
	*out = *in
	in.LivenessProbe.DeepCopyInto(&out.LivenessProbe)
	in.ReadinessProbe.DeepCopyInto(&out.ReadinessProbe)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Probes.
func (in *Probes) DeepCopy() *Probes {
	if in == nil {
		return nil
	}
	out := new(Probes)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Scheduling) DeepCopyInto(out *Scheduling) {
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
	if in.Tolerations != nil {
		in, out := &in.Tolerations, &out.Tolerations
		*out = make([]v1.Toleration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Scheduling.
func (in *Scheduling) DeepCopy() *Scheduling {
	if in == nil {
		return nil
	}
	out := new(Scheduling)
	in.DeepCopyInto(out)
	return out
}
