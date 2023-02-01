//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

// Code generated by openapi-gen. DO NOT EDIT.

// This file was autogenerated by openapi-gen. Do not edit it manually!

package v1beta1

import (
	common "k8s.io/kube-openapi/pkg/common"
	spec "k8s.io/kube-openapi/pkg/validation/spec"
)

func GetOpenAPIDefinitions(ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {
	return map[string]common.OpenAPIDefinition{
		"github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.Mattermost":     schema_mattermost_operator_apis_mattermost_v1beta1_Mattermost(ref),
		"github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.MattermostSpec": schema_mattermost_operator_apis_mattermost_v1beta1_MattermostSpec(ref),
	}
}

func schema_mattermost_operator_apis_mattermost_v1beta1_Mattermost(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "Mattermost is the Schema for the mattermosts API",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"kind": {
						SchemaProps: spec.SchemaProps{
							Description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"apiVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"metadata": {
						SchemaProps: spec.SchemaProps{
							Default: map[string]interface{}{},
							Ref:     ref("k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta"),
						},
					},
					"spec": {
						SchemaProps: spec.SchemaProps{
							Default: map[string]interface{}{},
							Ref:     ref("github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.MattermostSpec"),
						},
					},
					"status": {
						SchemaProps: spec.SchemaProps{
							Default: map[string]interface{}{},
							Ref:     ref("github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.MattermostStatus"),
						},
					},
				},
			},
		},
		Dependencies: []string{
			"github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.MattermostSpec", "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.MattermostStatus", "k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta"},
	}
}

func schema_mattermost_operator_apis_mattermost_v1beta1_MattermostSpec(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "MattermostSpec defines the desired state of Mattermost",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"size": {
						SchemaProps: spec.SchemaProps{
							Description: "Size defines the size of the Mattermost. This is typically specified in number of users. This will override replica and resource requests/limits appropriately for the provided number of users. This is a write-only field - its value is erased after setting appropriate values of resources. Accepted values are: 100users, 1000users, 5000users, 10000users, and 250000users. If replicas and resource requests/limits are not specified, and Size is not provided the configuration for 5000users will be applied. Setting 'Replicas', 'Scheduling.Resources', 'FileStore.Replicas', 'FileStore.Resource', 'Database.Replicas', or 'Database.Resources' will override the values set by Size. Setting new Size will override previous values regardless if set by Size or manually.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"image": {
						SchemaProps: spec.SchemaProps{
							Description: "Image defines the Mattermost Docker image.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"version": {
						SchemaProps: spec.SchemaProps{
							Description: "Version defines the Mattermost Docker image version.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"replicas": {
						SchemaProps: spec.SchemaProps{
							Description: "Replicas defines the number of replicas to use for the Mattermost app servers.",
							Type:        []string{"integer"},
							Format:      "int32",
						},
					},
					"mattermostEnv": {
						SchemaProps: spec.SchemaProps{
							Description: "Optional environment variables to set in the Mattermost application pods.",
							Type:        []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: map[string]interface{}{},
										Ref:     ref("k8s.io/api/core/v1.EnvVar"),
									},
								},
							},
						},
					},
					"licenseSecret": {
						SchemaProps: spec.SchemaProps{
							Description: "LicenseSecret is the name of the secret containing a Mattermost license.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"ingressName": {
						SchemaProps: spec.SchemaProps{
							Description: "IngressName defines the host to be used when creating the ingress rules. Deprecated: Use Spec.Ingress.Host instead.",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"ingressAnnotations": {
						SchemaProps: spec.SchemaProps{
							Description: "IngressAnnotations defines annotations passed to the Ingress associated with Mattermost. Deprecated: Use Spec.Ingress.Annotations.",
							Type:        []string{"object"},
							AdditionalProperties: &spec.SchemaOrBool{
								Allows: true,
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: "",
										Type:    []string{"string"},
										Format:  "",
									},
								},
							},
						},
					},
					"useIngressTLS": {
						SchemaProps: spec.SchemaProps{
							Description: "UseIngressTLS specifies whether TLS secret should be configured for Ingress. Deprecated: Use Spec.Ingress.TLSSecret.",
							Type:        []string{"boolean"},
							Format:      "",
						},
					},
					"useServiceLoadBalancer": {
						SchemaProps: spec.SchemaProps{
							Type:   []string{"boolean"},
							Format: "",
						},
					},
					"serviceAnnotations": {
						SchemaProps: spec.SchemaProps{
							Type: []string{"object"},
							AdditionalProperties: &spec.SchemaOrBool{
								Allows: true,
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: "",
										Type:    []string{"string"},
										Format:  "",
									},
								},
							},
						},
					},
					"resourceLabels": {
						SchemaProps: spec.SchemaProps{
							Type: []string{"object"},
							AdditionalProperties: &spec.SchemaOrBool{
								Allows: true,
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: "",
										Type:    []string{"string"},
										Format:  "",
									},
								},
							},
						},
					},
					"ingress": {
						SchemaProps: spec.SchemaProps{
							Description: "Ingress defines configuration for Ingress resource created by the Operator.",
							Ref:         ref("github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.Ingress"),
						},
					},
					"awsLoadBalancerController": {
						SchemaProps: spec.SchemaProps{
							Ref: ref("github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.AWSLoadBalancerController"),
						},
					},
					"volumes": {
						SchemaProps: spec.SchemaProps{
							Description: "Volumes allows for mounting volumes from various sources into the Mattermost application pods.",
							Type:        []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: map[string]interface{}{},
										Ref:     ref("k8s.io/api/core/v1.Volume"),
									},
								},
							},
						},
					},
					"volumeMounts": {
						SchemaProps: spec.SchemaProps{
							Description: "Defines additional volumeMounts to add to Mattermost application pods.",
							Type:        []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: map[string]interface{}{},
										Ref:     ref("k8s.io/api/core/v1.VolumeMount"),
									},
								},
							},
						},
					},
					"imagePullPolicy": {
						SchemaProps: spec.SchemaProps{
							Description: "Specify Mattermost deployment pull policy.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"imagePullSecrets": {
						SchemaProps: spec.SchemaProps{
							Description: "Specify Mattermost image pull secrets.",
							Type:        []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: map[string]interface{}{},
										Ref:     ref("k8s.io/api/core/v1.LocalObjectReference"),
									},
								},
							},
						},
					},
					"dnsConfig": {
						SchemaProps: spec.SchemaProps{
							Description: "Custom DNS configuration to use for the Mattermost Installation pods.",
							Ref:         ref("k8s.io/api/core/v1.PodDNSConfig"),
						},
					},
					"dnsPolicy": {
						SchemaProps: spec.SchemaProps{
							Description: "Custom DNS policy to use for the Mattermost Installation pods.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"database": {
						SchemaProps: spec.SchemaProps{
							Description: "External Services",
							Default:     map[string]interface{}{},
							Ref:         ref("github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.Database"),
						},
					},
					"fileStore": {
						SchemaProps: spec.SchemaProps{
							Default: map[string]interface{}{},
							Ref:     ref("github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.FileStore"),
						},
					},
					"elasticSearch": {
						SchemaProps: spec.SchemaProps{
							Default: map[string]interface{}{},
							Ref:     ref("github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.ElasticSearch"),
						},
					},
					"scheduling": {
						SchemaProps: spec.SchemaProps{
							Description: "Scheduling defines the configuration related to scheduling of the Mattermost pods as well as resource constraints. These settings generally don't need to be changed.",
							Default:     map[string]interface{}{},
							Ref:         ref("github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.Scheduling"),
						},
					},
					"probes": {
						SchemaProps: spec.SchemaProps{
							Description: "Probes defines configuration of liveness and readiness probe for Mattermost pods. These settings generally don't need to be changed.",
							Default:     map[string]interface{}{},
							Ref:         ref("github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.Probes"),
						},
					},
					"podTemplate": {
						SchemaProps: spec.SchemaProps{
							Description: "PodTemplate defines configuration for the template for Mattermost pods.",
							Ref:         ref("github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.PodTemplate"),
						},
					},
					"deploymentTemplate": {
						SchemaProps: spec.SchemaProps{
							Description: "DeploymentTemplate defines configuration for the template for Mattermost deployment.",
							Ref:         ref("github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.DeploymentTemplate"),
						},
					},
					"updateJob": {
						SchemaProps: spec.SchemaProps{
							Description: "UpdateJob defines configuration for the template for the update job.",
							Ref:         ref("github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.UpdateJob"),
						},
					},
					"podExtensions": {
						SchemaProps: spec.SchemaProps{
							Description: "PodExtensions specify custom extensions for Mattermost pods. This can be used for custom readiness checks etc. These settings generally don't need to be changed.",
							Default:     map[string]interface{}{},
							Ref:         ref("github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.PodExtensions"),
						},
					},
					"resourcePatch": {
						SchemaProps: spec.SchemaProps{
							Description: "ResourcePatch specifies JSON patches that can be applied to resources created by Mattermost Operator.\n\nWARNING: ResourcePatch is highly experimental and subject to change. Some patches may be impossible to perform or may impact the stability of Mattermost server.\n\nUse at your own risk when no other options are available.",
							Ref:         ref("github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.ResourcePatch"),
						},
					},
				},
			},
		},
		Dependencies: []string{
			"github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.AWSLoadBalancerController", "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.Database", "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.DeploymentTemplate", "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.ElasticSearch", "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.FileStore", "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.Ingress", "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.PodExtensions", "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.PodTemplate", "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.Probes", "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.ResourcePatch", "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.Scheduling", "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1.UpdateJob", "k8s.io/api/core/v1.EnvVar", "k8s.io/api/core/v1.LocalObjectReference", "k8s.io/api/core/v1.PodDNSConfig", "k8s.io/api/core/v1.Volume", "k8s.io/api/core/v1.VolumeMount"},
	}
}
