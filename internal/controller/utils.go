package controller

import (
	"fmt"
	"hash/fnv"

	"context"
	"strings"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

const (
	RNA_DEFAULT_NAMESPACE = "metalk8s-registry"
)

// getHash32Name returns a 32-bit hash of the input string - hexadecimal representation
func getHash32Name(input string) string {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(input)) //nolint:errcheck // Write() never returns an error for the FNV implementation
	hashSum := hasher.Sum32()
	return fmt.Sprintf("%x", hashSum)
}

// ChangeNamespace changes the namespace of the Registry Node Agent manifests
func (r *RegistryReconciler) ChangeNamespace(ctx context.Context, namespace string) {
	/*
	   Object type to modify Namespace:
	   * Namespace
	   * Certificate
	   * ValidatingWebhookConfiguration
	   * RoleBinding
	   * ClusterRoleBinding
	*/
	r.RNA.Namespaces[0].Name = namespace

	for _, cert := range r.RNA.Certificates {
		// We should find at least certificate for webhooks and metrics server
		dnsNames := []string{}
		// Change namespace in DNSNames
		for _, dnsName := range cert.Spec.DNSNames {
			newDNSName := strings.Replace(
				dnsName,
				fmt.Sprintf(".%s.svc", RNA_DEFAULT_NAMESPACE),
				fmt.Sprintf(".%s.svc", namespace),
				1,
			)
			dnsNames = append(dnsNames, newDNSName)
		}
		cert.Spec.DNSNames = dnsNames
	}

	for _, vwc := range r.RNA.ValidatingWebhookConfigurations {
		// Change namespace in annotations
		annotations := map[string]string{}
		for k, v := range vwc.GetAnnotations() {
			newValue := v
			if k == "cert-manager.io/inject-ca-from" {
				newValue = strings.Replace(
					v,
					RNA_DEFAULT_NAMESPACE,
					namespace,
					1,
				)
			}
			annotations[k] = newValue
		}
		vwc.SetAnnotations(annotations)

		webhooks := []admissionregistrationv1.ValidatingWebhook{}
		// Change namespace in admissionReviewVersions
		for _, webhook := range vwc.Webhooks {
			webhook.ClientConfig.Service.Namespace = namespace
			webhooks = append(webhooks, webhook)
		}
		vwc.Webhooks = webhooks
	}

	for _, roleBinding := range r.RNA.RoleBindings {
		subjects := []rbacv1.Subject{}
		// Change namespace in subjects
		for _, subject := range roleBinding.Subjects {
			if subject.Namespace == RNA_DEFAULT_NAMESPACE {
				subject.Namespace = namespace
			}
			subjects = append(subjects, subject)
		}
		roleBinding.Subjects = subjects
	}

	for _, clusterRoleBinding := range r.RNA.ClusterRoleBindings {
		subjects := []rbacv1.Subject{}
		// Change namespace in subjects
		for _, subject := range clusterRoleBinding.Subjects {
			if subject.Namespace == RNA_DEFAULT_NAMESPACE {
				subject.Namespace = namespace
			}
			subjects = append(subjects, subject)
		}
		clusterRoleBinding.Subjects = subjects
	}
}
