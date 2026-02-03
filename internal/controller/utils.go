package controller

import (
	"fmt"
	"hash/fnv"

	"context"
	"strings"
	"time"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
	"github.com/scality/metalk8s-registry-operator/internal/utils"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	ORGANIZATION_NAME          = "metalk8s"
	SSA_FIELD_OWNER_NAME       = "registry-operator"
	RNA_CA_NAME                = "metalk8s-registry-node-agent-ca"
	RNA_CA_SECRET_NAME         = "rna-ca-cert"
	RNA_CA_ISSUER_NAME         = "metalk8s-registry-node-agent-ca-issuer"
	RNA_SELFSIGNED_ISSUER_NAME = "metalk8s-registry-node-agent-selfsigned-issuer"
	RNA_SELFSIGNED_ISSUER_KIND = "Issuer"
	RNA_DEFAULT_NAMESPACE      = "metalk8s-registry"
)

// getHash32Name returns a 32-bit hash of the input string - hexadecimal representation
func getHash32Name(input string) string {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(input)) //nolint:errcheck // Write() never returns an error for the FNV implementation
	hashSum := hasher.Sum32()
	return fmt.Sprintf("%x", hashSum)
}

// ReconcileRNAGenericResources reconciles the generic resources of the Registry Node Agent structure
func (r *RegistryReconciler) ReconcileRNAGenericResources(ctx context.Context, registry *metalk8sv1alpha1.Registry) error {
	/*
	   Object type to reconcile from the Registry Node Agent structure:
	   * CustomResourceDefinition
	   * Namespace
	   * Certificate
	   * ValidatingWebhookConfiguration
	   * Role
	   * ClusterRole
	   * RoleBinding
	   * ClusterRoleBinding
	   * UnstructuredObjects
	*/
	log := logf.FromContext(ctx)
	var err error

	for _, crd := range r.RNA.CustomResourceDefinitions {
		err = r.Patch(ctx, crd, client.Apply, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
		if err != nil {
			log.V(1).Info("error patching CustomResourceDefinition", "name", crd.Name)
			return err
		}
		// The Patch action updates the struct with additional fields (such as managed fields)
		// We need to clean these fields
		utils.CleanResource(crd)
	}

	// If namespace has already been created, don't try to modify it
	for _, ns := range r.RNA.Namespaces {
		err = r.Patch(ctx, ns, client.Apply, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
		if err != nil {
			log.V(1).Info("error patching Namespace", "name", ns.Name)
			return err
		}
		// The Patch action updates the struct with additional fields (such as managed fields)
		// We need to clean these fields
		utils.CleanResource(ns)
	}

	for _, cert := range r.RNA.Certificates {
		cert.SetNamespace(*registry.Spec.Namespace)
		if err := controllerutil.SetControllerReference(registry, cert, r.Scheme); err != nil {
			log.V(1).Info("error setting controller reference for Certificate", "name", cert.Name)
			return err
		}
		err = r.Patch(ctx, cert, client.Apply, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
		if err != nil {
			log.V(1).Info("error patching Certificate", "name", cert.Name)
			return err
		}
		// The Patch action updates the struct with additional fields (such as managed fields)
		// We need to clean these fields
		utils.CleanResource(cert)
	}

	for _, vwc := range r.RNA.ValidatingWebhookConfigurations {
		if err := controllerutil.SetControllerReference(registry, vwc, r.Scheme); err != nil {
			log.V(1).Info("error setting controller reference for ValidatingWebhookConfiguration", "name", vwc.Name)
			return err
		}
		err = r.Patch(ctx, vwc, client.Apply, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
		if err != nil {
			log.V(1).Info("error patching ValidatingWebhookConfiguration", "name", vwc.Name)
			return err
		}
		// The Patch action updates the struct with additional fields (such as managed fields)
		// We need to clean these fields
		utils.CleanResource(vwc)
	}

	for _, role := range r.RNA.Roles {
		role.SetNamespace(*registry.Spec.Namespace)
		if err := controllerutil.SetControllerReference(registry, role, r.Scheme); err != nil {
			log.V(1).Info("error setting controller reference for Role", "name", role.Name)
			return err
		}
		err = r.Patch(ctx, role, client.Apply, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
		if err != nil {
			log.V(1).Info("error patching Role", "name", role.Name)
			return err
		}
		// The Patch action updates the struct with additional fields (such as managed fields)
		// We need to clean these fields
		utils.CleanResource(role)
	}

	for _, clusterRole := range r.RNA.ClusterRoles {
		if err := controllerutil.SetControllerReference(registry, clusterRole, r.Scheme); err != nil {
			log.V(1).Info("error setting controller reference for ClusterRole", "name", clusterRole.Name)
			return err
		}
		err = r.Patch(ctx, clusterRole, client.Apply, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
		if err != nil {
			log.V(1).Info("error patching ClusterRole", "name", clusterRole.Name)
			return err
		}
		// The Patch action updates the struct with additional fields (such as managed fields)
		// We need to clean these fields
		utils.CleanResource(clusterRole)
	}

	for _, roleBinding := range r.RNA.RoleBindings {
		roleBinding.SetNamespace(*registry.Spec.Namespace)
		if err := controllerutil.SetControllerReference(registry, roleBinding, r.Scheme); err != nil {
			log.V(1).Info("error setting controller reference for RoleBinding", "name", roleBinding.Name)
			return err
		}
		err = r.Patch(ctx, roleBinding, client.Apply, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
		if err != nil {
			log.V(1).Info("error patching RoleBinding", "name", roleBinding.Name)
			return err
		}
		// The Patch action updates the struct with additional fields (such as managed fields)
		// We need to clean these fields
		utils.CleanResource(roleBinding)
	}

	for _, clusterRoleBinding := range r.RNA.ClusterRoleBindings {
		clusterRoleBinding.SetNamespace(*registry.Spec.Namespace)
		if err := controllerutil.SetControllerReference(registry, clusterRoleBinding, r.Scheme); err != nil {
			log.V(1).Info("error setting controller reference for ClusterRoleBinding", "name", clusterRoleBinding.Name)
			return err
		}
		err = r.Patch(ctx, clusterRoleBinding, client.Apply, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
		if err != nil {
			log.V(1).Info("error patching ClusterRoleBinding", "name", clusterRoleBinding.Name)
			return err
		}
		// The Patch action updates the struct with additional fields (such as managed fields)
		// We need to clean these fields
		utils.CleanResource(clusterRoleBinding)
	}

	for _, obj := range r.RNA.UnstructuredObjects {
		obj.SetNamespace(*registry.Spec.Namespace)
		if err := controllerutil.SetControllerReference(registry, obj, r.Scheme); err != nil {
			log.V(1).Info("error setting controller reference for UnstructuredObject", "name", obj.GetName())
			return err
		}
		err = r.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
		if err != nil {
			log.V(1).Info("error patching UnstructuredObject", "name", obj.GetName())
			return err
		}
		// The Patch action updates the struct with additional fields (such as managed fields)
		// We need to clean these fields
		utils.CleanResource(obj)
	}

	return nil
}

// ReconcileRNACACertificate reconciles a 1-year self-signed CA certificate to sign TLS Certificate for internal server
func (r *RegistryReconciler) ReconcileRNACACertificate(ctx context.Context, registryNamespace string, registry *metalk8sv1alpha1.Registry) error {
	registryNodeAgentCACertificate := &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      RNA_CA_NAME,
			Namespace: registryNamespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, registryNodeAgentCACertificate, func() error {
		err := controllerutil.SetControllerReference(registry, registryNodeAgentCACertificate, r.Scheme)
		if err != nil {
			return err
		}
		registryNodeAgentCACertificate.Spec.IsCA = true
		registryNodeAgentCACertificate.Spec.SecretName = RNA_CA_SECRET_NAME
		registryNodeAgentCACertificate.Spec.IssuerRef = cmmetav1.IssuerReference{
			Name: RNA_SELFSIGNED_ISSUER_NAME,
			Kind: RNA_SELFSIGNED_ISSUER_KIND,
		}
		registryNodeAgentCACertificate.Spec.Subject = &cmv1.X509Subject{
			Organizations: []string{ORGANIZATION_NAME},
		}
		registryNodeAgentCACertificate.Spec.CommonName = RNA_CA_NAME
		registryNodeAgentCACertificate.Spec.Duration = &metav1.Duration{Duration: 365 * 24 * time.Hour}
		registryNodeAgentCACertificate.Spec.Usages = []cmv1.KeyUsage{
			cmv1.UsageCertSign,
			cmv1.UsageCRLSign,
			cmv1.UsageDigitalSignature,
		}
		return nil
	})

	return err
}

// ReconcileRNACAIssuer reconciles an Issuer linked to CA Certificate
func (r *RegistryReconciler) ReconcileRNACAIssuer(ctx context.Context, registryNamespace string, registry *metalk8sv1alpha1.Registry) error {
	registryNodeAgentCAIssuer := &cmv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      RNA_CA_ISSUER_NAME,
			Namespace: registryNamespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, registryNodeAgentCAIssuer, func() error {
		err := controllerutil.SetControllerReference(registry, registryNodeAgentCAIssuer, r.Scheme)
		if err != nil {
			return err
		}
		registryNodeAgentCAIssuer.Spec.IssuerConfig = cmv1.IssuerConfig{
			CA: &cmv1.CAIssuer{
				SecretName: RNA_CA_SECRET_NAME,
			},
		}
		return nil
	})
	return err
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
