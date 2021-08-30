package resources

import corev1 "k8s.io/api/core/v1"

// CopyServiceEmptyAutoAssignedFields copies fields from an existing service that are populated automatically
// by Kubernetes when not provided to avoid issues with changing immutable fields.
func CopyServiceEmptyAutoAssignedFields(desired, actual *corev1.Service) {
	if desired.Spec.ClusterIP == "" {
		desired.Spec.ClusterIP = actual.Spec.ClusterIP
	}
	if len(desired.Spec.ClusterIPs) == 0 {
		desired.Spec.ClusterIPs = actual.Spec.ClusterIPs
	}

	// If both services are of type LoadBalancer and LoadBalancerIP is not set on new one - copy from existing.
	if desired.Spec.Type == corev1.ServiceTypeLoadBalancer && actual.Spec.Type == corev1.ServiceTypeLoadBalancer && desired.Spec.LoadBalancerIP == "" {
		desired.Spec.LoadBalancerIP = actual.Spec.LoadBalancerIP
	}
}
