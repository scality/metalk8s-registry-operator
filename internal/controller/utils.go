package controller

import (
	"fmt"
	"hash/fnv"
	"slices"

	"context"
	"strings"
	"time"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
	"github.com/scality/metalk8s-registry-operator/internal/utils"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	ORGANIZATION_NAME                      = "metalk8s"
	SSA_FIELD_OWNER_NAME                   = "registry-operator"
	CONTAINERD_MIRROR_CONFIGMAP_NAME       = "metalk8s-registry-containerd-mirror"
	CONTAINERD_MIRROR_SYNC_DAEMONSET_NAME  = "metalk8s-registry-containerd-mirror-sync"
	CERTS_D_SUBDIR_ANNOTATION              = "registry.metalk8s.scality.com/certs-d-subdir"
	CERTS_D_SUBDIR_VALUE                   = "_default"
	MIRROR_HOSTS_TOML_KEY                  = "hosts.toml"
	MIRROR_CA_KEY                          = "ca.crt"
	RNA_CA_NAME                            = "metalk8s-registry-node-agent-ca"
	RNA_CA_SECRET_NAME                     = "rna-ca-cert"
	RNA_CA_ISSUER_NAME                     = "metalk8s-registry-node-agent-ca-issuer"
	RNA_CA_ISSUER_KIND                     = "Issuer"
	RNA_SELFSIGNED_ISSUER_NAME             = "metalk8s-registry-node-agent-selfsigned-issuer"
	RNA_SELFSIGNED_ISSUER_KIND             = "Issuer"
	RNA_STATEFULSET_PREFIX                 = "metalk8s-registry-node-agent"
	RS_STATEFULSET_PREFIX                  = "metalk8s-registry-server"
	RNA_INTERNAL_SERVER_CERTIFICATE_PREFIX = "rna-internal-server"
	RNA_INTERNAL_SERVER_CERTIFICATE_CN     = "rna-internal-server"
	RNA_EXTERNAL_SERVER_CERTIFICATE_PREFIX = "rna-external-server"
	RNA_EXTERNAL_SERVER_CERTIFICATE_CN     = "rna-external-server"
	RNA_INTERNAL_CLIENT_CERTIFICATE_PREFIX = "rna-internal-client"
	RNA_EXTERNAL_CLIENT_CERTIFICATE_PREFIX = "rna-external-client"
	RS_EXTERNAL_SERVER_CERTIFICATE_PREFIX  = "rs-external-server"
	RS_EXTERNAL_SERVER_CERTIFICATE_CN      = "rs-external-server"
	TLS_CLIENT_INTERNAL_CERTS_NAME         = "tls-client-intern-certs"
	TLS_SERVER_INTERNAL_CERTS_NAME         = "tls-server-intern-certs"
	TLS_SERVER_EXTERNAL_CERTS_NAME         = "tls-server-extern-certs"
	TLS_CLIENT_EXTERNAL_CERTS_NAME         = "tls-client-extern-certs"
	RS_SERVICE_NAME                        = "metalk8s-registry-server"
	RS_SERVER_PORT                         = 5000
	MTLS_CA_HASH_ANNOTATION_KEY            = "registry.metalk8s.scality.com/mtls-ca-hash"
)

// getHash32Name returns a 32-bit hash of the input string - hexadecimal representation
func getHash32Name(input string) string {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(input)) //nolint:errcheck // Write() never returns an error for the FNV implementation
	hashSum := hasher.Sum32()
	return fmt.Sprintf("%08x", hashSum)
}

// safeNodeName returns a token derived from the node name that is safe to embed
// in the per-node resource names: no dots and short enough to keep
// "<prefix>-<token>" (plus the StatefulSet pod ordinal and controller-revision
// suffixes) within the 63-character DNS label budget. Short names without dots
// are kept as-is; anything else is sanitized, truncated and suffixed with a
// hash of the original name to remain unique and deterministic.
// Never use it for the "node" label, which carries the real node name.
func safeNodeName(nodeName string) string {
	const maxTokenLen = 23
	if len(nodeName) <= maxTokenLen && !strings.Contains(nodeName, ".") {
		return nodeName
	}
	hash := getHash32Name(nodeName)
	sanitized := strings.ReplaceAll(nodeName, ".", "-")
	if maxBase := maxTokenLen - len(hash) - 1; len(sanitized) > maxBase {
		sanitized = sanitized[:maxBase]
	}
	return strings.TrimRight(sanitized, "-") + "-" + hash
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
	   * ServiceMonitor
	   * UnstructuredObjects
	*/
	log := logf.FromContext(ctx)
	var err error

	for _, crd := range r.RNA.CustomResourceDefinitions {
		err = r.Patch(ctx, crd, utils.ApplyPatch, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
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
		err = r.Patch(ctx, ns, utils.ApplyPatch, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
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
		err = r.Patch(ctx, cert, utils.ApplyPatch, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
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
		err = r.Patch(ctx, vwc, utils.ApplyPatch, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
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
		err = r.Patch(ctx, role, utils.ApplyPatch, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
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
		err = r.Patch(ctx, clusterRole, utils.ApplyPatch, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
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
		err = r.Patch(ctx, roleBinding, utils.ApplyPatch, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
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
		err = r.Patch(ctx, clusterRoleBinding, utils.ApplyPatch, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
		if err != nil {
			log.V(1).Info("error patching ClusterRoleBinding", "name", clusterRoleBinding.Name)
			return err
		}
		// The Patch action updates the struct with additional fields (such as managed fields)
		// We need to clean these fields
		utils.CleanResource(clusterRoleBinding)
	}

	if err = r.reconcileServiceMonitors(ctx, registry); err != nil {
		return err
	}

	for _, obj := range r.RNA.UnstructuredObjects {
		obj.SetNamespace(*registry.Spec.Namespace)
		if err := controllerutil.SetControllerReference(registry, obj, r.Scheme); err != nil {
			log.V(1).Info("error setting controller reference for UnstructuredObject", "name", obj.GetName())
			return err
		}
		err = r.Patch(ctx, obj, utils.ApplyPatch, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
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

// reconcileServiceMonitors applies the Registry Node Agent ServiceMonitors, if their CRD is established
func (r *RegistryReconciler) reconcileServiceMonitors(ctx context.Context, registry *metalk8sv1alpha1.Registry) error {
	log := logf.FromContext(ctx)

	watched, err := r.ensureServiceMonitorWatch(ctx)
	if err != nil {
		return err
	}
	if !watched {
		if len(r.RNA.ServiceMonitors) > 0 {
			log.V(1).Info("ServiceMonitor CRD not established, skipping ServiceMonitors")
		}
		return nil
	}

	for _, serviceMonitor := range r.RNA.ServiceMonitors {
		serviceMonitor.SetNamespace(*registry.Spec.Namespace)
		if err := controllerutil.SetControllerReference(registry, serviceMonitor, r.Scheme); err != nil {
			log.V(1).Info("error setting controller reference for ServiceMonitor", "name", serviceMonitor.Name)
			return err
		}
		err := r.Patch(ctx, serviceMonitor, utils.ApplyPatch, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
		if err != nil {
			log.V(1).Info("error patching ServiceMonitor", "name", serviceMonitor.Name)
			return err
		}
		// The Patch action updates the struct with additional fields (such as managed fields)
		// We need to clean these fields
		utils.CleanResource(serviceMonitor)
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

// ReconcileRNAExternalClientCACertificate reconciles a namespaced copy of the CA certificate defined in the Registry (spec.agent.authentication.mtls.caSecretRef)
// It is used to sign the mTLS Certificate used to upload ISO files
func (r *RegistryReconciler) ReconcileRNAExternalClientCACertificate(ctx context.Context, registryNamespace string, registry *metalk8sv1alpha1.Registry, mTLSCAsHash *string) error {
	caSecretRef := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      registry.Spec.Agent.Authentication.MTLS.CASecretRef.Name,
		Namespace: registry.Spec.Agent.Authentication.MTLS.CASecretRef.Namespace,
	}, caSecretRef); err != nil {
		return err
	}

	// Generate the hash of the mTLS CA certificate
	*mTLSCAsHash = getHash32Name(string(caSecretRef.Data["ca.crt"]))
	externalClientCASecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      RNA_EXTERNAL_CLIENT_CERTIFICATE_PREFIX,
			Namespace: registryNamespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, externalClientCASecret, func() error {
		err := controllerutil.SetControllerReference(registry, externalClientCASecret, r.Scheme)
		if err != nil {
			return err
		}

		// Set the hash of the mTLS CA certificate(s) as an annotation
		if externalClientCASecret.Annotations == nil {
			externalClientCASecret.Annotations = map[string]string{}
		}
		externalClientCASecret.Annotations[MTLS_CA_HASH_ANNOTATION_KEY] = *mTLSCAsHash

		externalClientCASecret.Data = map[string][]byte{
			"ca.crt": caSecretRef.Data["ca.crt"],
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
	   * ServiceMonitor
	*/
	r.RNA.Namespaces[0].Name = namespace

	for _, cert := range r.RNA.Certificates {
		// We should find at least certificate for webhooks and metrics server
		dnsNames := make([]string, 0, len(cert.Spec.DNSNames))
		// Change namespace in DNSNames
		for _, dnsName := range cert.Spec.DNSNames {
			newDNSName := strings.Replace(
				dnsName,
				fmt.Sprintf(".%s.svc", metalk8sv1alpha1.DEFAULT_NAMESPACE),
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
					metalk8sv1alpha1.DEFAULT_NAMESPACE+"/",
					namespace+"/",
					1,
				)
			}
			annotations[k] = newValue
		}
		vwc.SetAnnotations(annotations)

		webhooks := make([]admissionregistrationv1.ValidatingWebhook, 0, len(vwc.Webhooks))
		// Change namespace in admissionReviewVersions
		for _, webhook := range vwc.Webhooks {
			webhook.ClientConfig.Service.Namespace = namespace
			webhooks = append(webhooks, webhook)
		}
		vwc.Webhooks = webhooks
	}

	for _, roleBinding := range r.RNA.RoleBindings {
		subjects := make([]rbacv1.Subject, 0, len(roleBinding.Subjects))
		// Change namespace in subjects
		for _, subject := range roleBinding.Subjects {
			if subject.Namespace == metalk8sv1alpha1.DEFAULT_NAMESPACE {
				subject.Namespace = namespace
			}
			subjects = append(subjects, subject)
		}
		roleBinding.Subjects = subjects
	}

	for _, clusterRoleBinding := range r.RNA.ClusterRoleBindings {
		subjects := make([]rbacv1.Subject, 0, len(clusterRoleBinding.Subjects))
		// Change namespace in subjects
		for _, subject := range clusterRoleBinding.Subjects {
			if subject.Namespace == metalk8sv1alpha1.DEFAULT_NAMESPACE {
				subject.Namespace = namespace
			}
			subjects = append(subjects, subject)
		}
		clusterRoleBinding.Subjects = subjects
	}

	for _, serviceMonitor := range r.RNA.ServiceMonitors {
		endpoints := make([]monitoringv1.Endpoint, 0, len(serviceMonitor.Spec.Endpoints))
		// Change namespace in TLS server name
		for _, endpoint := range serviceMonitor.Spec.Endpoints {
			if endpoint.TLSConfig != nil && endpoint.TLSConfig.ServerName != nil {
				serverName := strings.Replace(
					*endpoint.TLSConfig.ServerName,
					fmt.Sprintf(".%s.svc", metalk8sv1alpha1.DEFAULT_NAMESPACE),
					fmt.Sprintf(".%s.svc", namespace),
					1,
				)
				endpoint.TLSConfig.ServerName = &serverName
			}
			endpoints = append(endpoints, endpoint)
		}
		serviceMonitor.Spec.Endpoints = endpoints
	}
}

type componentSts struct {
	sts *appsv1.StatefulSet
}

func (cpt componentSts) setAffinity(nodeName string) {
	cpt.sts.Spec.Template.Spec.Affinity = &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchFields: []corev1.NodeSelectorRequirement{
							{
								Key:      "metadata.name",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{nodeName},
							},
						},
					},
				},
			},
		},
	}
}

func (cpt componentSts) setNodeLabel(nodeName string) {
	cpt.sts.Spec.Template.Labels[NODE_LABEL_KEY] = nodeName
}

func (cpt componentSts) setPodTemplateAnnotation(mTLSCAsHash string) {
	if cpt.sts.Spec.Template.Annotations == nil {
		cpt.sts.Spec.Template.Annotations = map[string]string{}
	}
	cpt.sts.Spec.Template.Annotations[MTLS_CA_HASH_ANNOTATION_KEY] = mTLSCAsHash
}

// isStatefulSetTerminating reports whether the StatefulSet exists and is being deleted
func (r *RegistryReconciler) isStatefulSetTerminating(ctx context.Context, name string, namespace string) (bool, error) {
	sts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, sts); err != nil {
		return false, client.IgnoreNotFound(err)
	}
	return !sts.DeletionTimestamp.IsZero(), nil
}

/*
	Beyond this point, the functions are specific to one Registry Node Agent instance on a Node:
*/

// ReconcileRNAStatefulSet reconciles a Registry Node Agent as a StatefulSet on the specified node
func (r *RegistryReconciler) ReconcileRNAStatefulSet(ctx context.Context, registryNamespace string, nodeName string, mTLSCAsHash string, registry *metalk8sv1alpha1.Registry) error {
	nodeToken := safeNodeName(nodeName)
	stsName := fmt.Sprintf("%s-%s", RNA_STATEFULSET_PREFIX, nodeToken)

	// A StatefulSet under deletion must not be applied again: applying it without
	// our finalizer would remove it before NodeSolutionArchives are cleaned up.
	if terminating, err := r.isStatefulSetTerminating(ctx, stsName, registryNamespace); err != nil || terminating {
		return err
	}

	// Always start from the embedded manifest so that template changes reach existing StatefulSets
	registryNodeAgentStatefulSet := componentSts{r.RNA.StatefulSets[0].DeepCopy()}

	// Set metadata on StatefulSet
	registryNodeAgentStatefulSet.sts.SetName(stsName)
	registryNodeAgentStatefulSet.sts.SetNamespace(registryNamespace)
	registryNodeAgentStatefulSet.sts.Labels[NODE_LABEL_KEY] = nodeName
	if err := controllerutil.SetControllerReference(registry, registryNodeAgentStatefulSet.sts, r.Scheme); err != nil {
		return err
	}

	// Modify replica to 1
	registryNodeAgentStatefulSet.sts.Spec.Replicas = ptr.To(int32(1))

	// Set affinity to the specified node to ensure the StatefulSet is scheduled on the specified node
	registryNodeAgentStatefulSet.setAffinity(nodeName)

	// Set Registry/Image:Tag
	registryNodeAgentStatefulSet.setRNAImageTag(registry)

	// Set ImagePullPolicy, if defined
	if registry.Spec.Agent.Image.PullPolicy != nil {
		registryNodeAgentStatefulSet.sts.Spec.Template.Spec.Containers[0].ImagePullPolicy = *registry.Spec.Agent.Image.PullPolicy
	}

	// Set ImagePullSecrets, if defined
	if registry.Spec.Agent.Image.PullSecrets != nil {
		registryNodeAgentStatefulSet.sts.Spec.Template.Spec.ImagePullSecrets = registry.Spec.Agent.Image.PullSecrets
	}

	// Set Node label on Pod
	registryNodeAgentStatefulSet.setNodeLabel(nodeName)
	registryNodeAgentStatefulSet.setPodTemplateAnnotation(mTLSCAsHash)

	// Set Volumes (archives, solutions, dev, TLS/mTLS Certificates, webhook-certs)
	if err := registryNodeAgentStatefulSet.setRNAVolumes(registry, nodeName); err != nil {
		return err
	}

	// Set Environment Variables (DOWNLOAD_HOST, LOGLEVEL)
	registryNodeAgentStatefulSet.setRNAEnvVariables(nodeName, registryNamespace, registry)

	controllerutil.AddFinalizer(registryNodeAgentStatefulSet.sts, FINALIZER_NAME)

	return r.Patch(ctx, registryNodeAgentStatefulSet.sts, utils.ApplyPatch, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
}

// ReconcileRNAService reconciles a Registry Node Agent service
func (r *RegistryReconciler) ReconcileRNAService(ctx context.Context, registryNamespace string, nodeName string, registry *metalk8sv1alpha1.Registry) error {
	nodeToken := safeNodeName(nodeName)
	registryNodeAgentService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", RNA_INTERNAL_SERVER_CERTIFICATE_CN, nodeToken),
			Namespace: registryNamespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, registryNodeAgentService, func() error {
		registryNodeAgentService.SetLabels(map[string]string{
			REG_APP_LABEL_KEY: RNA_APP_LABEL_VALUE,
			NODE_LABEL_KEY:    nodeName,
		})
		err := controllerutil.SetControllerReference(registry, registryNodeAgentService, r.Scheme)
		if err != nil {
			return err
		}

		registryNodeAgentService.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "tcp-download",
				Port:       5002,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromInt32(5002),
			},
		}
		registryNodeAgentService.Spec.Selector = map[string]string{
			REG_APP_LABEL_KEY: RNA_APP_LABEL_VALUE,
			"control-plane":   "controller-manager",
			NODE_LABEL_KEY:    nodeName,
		}
		registryNodeAgentService.Spec.Type = corev1.ServiceTypeClusterIP

		return nil
	})
	return err
}

// ReconcileRNAInternalServerCertificate reconciles an internal server certificate for the Registry Node Agent
func (r *RegistryReconciler) ReconcileRNAInternalServerCertificate(ctx context.Context, registryNamespace string, nodeName string, registry *metalk8sv1alpha1.Registry) error {
	nodeToken := safeNodeName(nodeName)
	registryNodeAgentServerCertificate := &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", RNA_INTERNAL_SERVER_CERTIFICATE_PREFIX, nodeToken),
			Namespace: registryNamespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, registryNodeAgentServerCertificate, func() error {
		registryNodeAgentServerCertificate.SetLabels(map[string]string{
			REG_APP_LABEL_KEY: RNA_APP_LABEL_VALUE,
			NODE_LABEL_KEY:    nodeName,
		})
		err := controllerutil.SetControllerReference(registry, registryNodeAgentServerCertificate, r.Scheme)
		if err != nil {
			return err
		}
		registryNodeAgentServerCertificate.Spec.SecretName = fmt.Sprintf("%s-%s", RNA_INTERNAL_SERVER_CERTIFICATE_PREFIX, nodeToken)
		registryNodeAgentServerCertificate.Spec.IssuerRef = cmmetav1.IssuerReference{
			Name: RNA_CA_ISSUER_NAME,
			Kind: RNA_CA_ISSUER_KIND,
		}
		registryNodeAgentServerCertificate.Spec.CommonName = fmt.Sprintf("%s-%s", RNA_INTERNAL_SERVER_CERTIFICATE_CN, nodeToken)
		registryNodeAgentServerCertificate.Spec.DNSNames = []string{
			fmt.Sprintf("%s-%s", RNA_INTERNAL_SERVER_CERTIFICATE_CN, nodeToken),
			fmt.Sprintf("%s-%s.%s", RNA_INTERNAL_SERVER_CERTIFICATE_CN, nodeToken, registryNamespace),
			fmt.Sprintf("%s-%s.%s.svc", RNA_INTERNAL_SERVER_CERTIFICATE_CN, nodeToken, registryNamespace),
			fmt.Sprintf("%s-%s.%s.svc.cluster.local", RNA_INTERNAL_SERVER_CERTIFICATE_CN, nodeToken, registryNamespace),
		}
		registryNodeAgentServerCertificate.Spec.Usages = []cmv1.KeyUsage{
			cmv1.UsageKeyEncipherment,
			cmv1.UsageDigitalSignature,
			cmv1.UsageServerAuth,
		}
		return nil
	})
	return err
}

// ReconcileRNAExternalServerCertificate reconciles an external server certificate for the Registry Node Agent
func (r *RegistryReconciler) ReconcileRNAExternalServerCertificate(ctx context.Context, registryNamespace string, nodeName string, nodeIP string, registry *metalk8sv1alpha1.Registry) error {
	nodeToken := safeNodeName(nodeName)
	registryNodeAgentServerCertificate := &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", RNA_EXTERNAL_SERVER_CERTIFICATE_PREFIX, nodeToken),
			Namespace: registryNamespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, registryNodeAgentServerCertificate, func() error {
		registryNodeAgentServerCertificate.SetLabels(map[string]string{
			REG_APP_LABEL_KEY: RNA_APP_LABEL_VALUE,
			NODE_LABEL_KEY:    nodeName,
		})
		err := controllerutil.SetControllerReference(registry, registryNodeAgentServerCertificate, r.Scheme)
		if err != nil {
			return err
		}
		registryNodeAgentServerCertificate.Spec.SecretName = fmt.Sprintf("%s-%s", RNA_EXTERNAL_SERVER_CERTIFICATE_PREFIX, nodeToken)
		registryNodeAgentServerCertificate.Spec.IssuerRef = cmmetav1.IssuerReference{
			Name: registry.Spec.Agent.CertificateIssuerRef.Name,
			Kind: registry.Spec.Agent.CertificateIssuerRef.Kind,
		}
		registryNodeAgentServerCertificate.Spec.CommonName = fmt.Sprintf("%s-%s", RNA_EXTERNAL_SERVER_CERTIFICATE_CN, nodeToken)
		registryNodeAgentServerCertificate.Spec.IPAddresses = []string{
			nodeIP,
		}
		registryNodeAgentServerCertificate.Spec.Usages = []cmv1.KeyUsage{
			cmv1.UsageKeyEncipherment,
			cmv1.UsageDigitalSignature,
			cmv1.UsageServerAuth,
		}
		return nil
	})
	return err
}

// ReconcileRNAClientCertificate reconciles a client certificate for the Registry Node Agent
func (r *RegistryReconciler) ReconcileRNAClientCertificate(ctx context.Context, registryNamespace string, nodeName string, registry *metalk8sv1alpha1.Registry) error {
	nodeToken := safeNodeName(nodeName)
	registryNodeAgentCertificate := &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", RNA_INTERNAL_CLIENT_CERTIFICATE_PREFIX, nodeToken),
			Namespace: registryNamespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, registryNodeAgentCertificate, func() error {
		registryNodeAgentCertificate.SetLabels(map[string]string{
			REG_APP_LABEL_KEY: RNA_APP_LABEL_VALUE,
			NODE_LABEL_KEY:    nodeName,
		})
		err := controllerutil.SetControllerReference(registry, registryNodeAgentCertificate, r.Scheme)
		if err != nil {
			return err
		}
		registryNodeAgentCertificate.Spec.SecretName = fmt.Sprintf("%s-%s", RNA_INTERNAL_CLIENT_CERTIFICATE_PREFIX, nodeToken)
		registryNodeAgentCertificate.Spec.IssuerRef = cmmetav1.IssuerReference{
			Group: "cert-manager.io",
			Kind:  RNA_CA_ISSUER_KIND,
			Name:  RNA_CA_ISSUER_NAME,
		}
		registryNodeAgentCertificate.Spec.CommonName = fmt.Sprintf("%s-%s", RNA_INTERNAL_CLIENT_CERTIFICATE_PREFIX, nodeToken)
		registryNodeAgentCertificate.Spec.Usages = []cmv1.KeyUsage{
			cmv1.UsageKeyEncipherment,
			cmv1.UsageDigitalSignature,
			cmv1.UsageClientAuth,
		}
		return nil
	})
	return err
}

func (cpt componentSts) setRNAImageTag(registry *metalk8sv1alpha1.Registry) {
	targetedImage := registry.Spec.Agent.Image.GetImage()
	cpt.sts.Spec.Template.Spec.Containers[0].Image = targetedImage
	cpt.sts.Spec.Template.Spec.InitContainers[0].Image = targetedImage
}

func (cpt componentSts) setRNAVolumes(registry *metalk8sv1alpha1.Registry, nodeName string) error {
	nodeToken := safeNodeName(nodeName)
	volumesMapping := make(map[string]int)
	for id, volume := range cpt.sts.Spec.Template.Spec.Volumes {
		volumesMapping[volume.Name] = id
	}

	// Only override the fields the operator owns (hostPath path, secret name):
	// everything else (hostPath type, defaultMode, ...) comes from the base manifest.
	setHostPathVolumePath := func(volumeName string, path string) error {
		idVol, exists := volumesMapping[volumeName]
		if !exists {
			return fmt.Errorf("volume %s not found", volumeName)
		}
		hostPath := cpt.sts.Spec.Template.Spec.Volumes[idVol].HostPath
		if hostPath == nil {
			return fmt.Errorf("volume %s is not a hostPath volume", volumeName)
		}
		hostPath.Path = path
		return nil
	}
	setSecretVolumeName := func(volumeName string, secretName string) error {
		idVol, exists := volumesMapping[volumeName]
		if !exists {
			return fmt.Errorf("volume %s not found", volumeName)
		}
		secret := cpt.sts.Spec.Template.Spec.Volumes[idVol].Secret
		if secret == nil {
			return fmt.Errorf("volume %s is not a secret volume", volumeName)
		}
		secret.SecretName = secretName
		return nil
	}

	// metalk8s-registry-node-agent-archives: where the ISO files are stored
	if err := setHostPathVolumePath("metalk8s-registry-node-agent-archives", *registry.Spec.ArchivesPath); err != nil {
		return err
	}

	// metalk8s-registry-node-agent-solutions: where the solutions are mounted
	if err := setHostPathVolumePath("metalk8s-registry-node-agent-solutions", *registry.Spec.SolutionsPath); err != nil {
		return err
	}

	// External Server TLS Certificate
	if err := setSecretVolumeName(TLS_SERVER_EXTERNAL_CERTS_NAME,
		fmt.Sprintf("%s-%s", RNA_EXTERNAL_SERVER_CERTIFICATE_PREFIX, nodeToken)); err != nil {
		return err
	}

	// External Client CA mTLS Certificate
	if err := setSecretVolumeName(TLS_CLIENT_EXTERNAL_CERTS_NAME, RNA_EXTERNAL_CLIENT_CERTIFICATE_PREFIX); err != nil {
		return err
	}

	// Internal Server TLS Certificate
	if err := setSecretVolumeName(TLS_SERVER_INTERNAL_CERTS_NAME,
		fmt.Sprintf("%s-%s", RNA_INTERNAL_SERVER_CERTIFICATE_PREFIX, nodeToken)); err != nil {
		return err
	}

	// Internal Client mTLS Certificate
	if err := setSecretVolumeName(TLS_CLIENT_INTERNAL_CERTS_NAME,
		fmt.Sprintf("%s-%s", RNA_INTERNAL_CLIENT_CERTIFICATE_PREFIX, nodeToken)); err != nil {
		return err
	}

	return nil
}

func (cpt componentSts) setRNAEnvVariables(nodeName string, registryNamespace string, registry *metalk8sv1alpha1.Registry) {
	nodeToken := safeNodeName(nodeName)
	environmentMapping := make(map[string]int)
	for id, env := range cpt.sts.Spec.Template.Spec.Containers[0].Env {
		environmentMapping[env.Name] = id
	}

	// Change DOWNLOAD_HOST
	downloadHost := corev1.EnvVar{
		Name:  "DOWNLOAD_HOST",
		Value: fmt.Sprintf("%s-%s.%s.svc", RNA_INTERNAL_SERVER_CERTIFICATE_CN, nodeToken, registryNamespace),
	}
	if idx, exists := environmentMapping["DOWNLOAD_HOST"]; !exists {
		cpt.sts.Spec.Template.Spec.Containers[0].Env = append(cpt.sts.Spec.Template.Spec.Containers[0].Env, downloadHost)
	} else {
		cpt.sts.Spec.Template.Spec.Containers[0].Env[idx] = downloadHost
	}

	// Change LOGGER_LOG_LEVEL
	logLevel := corev1.EnvVar{
		Name:  "LOGGER_LOG_LEVEL",
		Value: *registry.Spec.LogLevel,
	}
	if idx, exists := environmentMapping["LOGGER_LOG_LEVEL"]; !exists {
		cpt.sts.Spec.Template.Spec.Containers[0].Env = append(cpt.sts.Spec.Template.Spec.Containers[0].Env, logLevel)
	} else {
		cpt.sts.Spec.Template.Spec.Containers[0].Env[idx] = logLevel
	}
}

/*
	Beyond this point, the functions are specific to one Registry Server instance on a Node:
*/

// ReconcileRSStatefulSet reconciles a Registry Server as a StatefulSet on the specified node
func (r *RegistryReconciler) ReconcileRSStatefulSet(ctx context.Context, registryNamespace string, nodeName string, nodeIP string, registry *metalk8sv1alpha1.Registry) error {
	nodeToken := safeNodeName(nodeName)
	stsName := fmt.Sprintf("%s-%s", RS_STATEFULSET_PREFIX, nodeToken)

	// A StatefulSet under deletion must not be applied again
	if terminating, err := r.isStatefulSetTerminating(ctx, stsName, registryNamespace); err != nil || terminating {
		return err
	}

	registryServerStatefulSet := componentSts{r.RS.StatefulSets[0].DeepCopy()}

	// Set metadata on StatefulSet
	registryServerStatefulSet.sts.SetName(stsName)
	registryServerStatefulSet.sts.SetNamespace(registryNamespace)
	registryServerStatefulSet.sts.Labels[NODE_LABEL_KEY] = nodeName
	if err := controllerutil.SetControllerReference(registry, registryServerStatefulSet.sts, r.Scheme); err != nil {
		return err
	}

	// Set replica to 1
	registryServerStatefulSet.sts.Spec.Replicas = ptr.To(int32(1))

	// Set affinity to the specified node to ensure the StatefulSet is scheduled on the specified node
	registryServerStatefulSet.setAffinity(nodeName)

	// Set Registry/Image:Tag
	registryServerStatefulSet.setRSImageTag(registry)

	// Set ImagePullPolicy, if defined
	if registry.Spec.Server.Image.PullPolicy != nil {
		registryServerStatefulSet.sts.Spec.Template.Spec.Containers[0].ImagePullPolicy = *registry.Spec.Server.Image.PullPolicy
	}

	// Set ImagePullSecrets, if defined
	if registry.Spec.Server.Image.PullSecrets != nil {
		registryServerStatefulSet.sts.Spec.Template.Spec.ImagePullSecrets = registry.Spec.Server.Image.PullSecrets
	}

	// Set Node label on Pod
	registryServerStatefulSet.setNodeLabel(nodeName)

	// Set Volumes (solutions, TLS Certificate)
	if err := registryServerStatefulSet.setRSVolumes(registry, nodeName); err != nil {
		return err
	}

	// Set Environment Variables (LOGLEVEL, HTTP_ADDR)
	registryServerStatefulSet.setRSEnvVariables(registry, nodeIP)

	return r.Patch(ctx, registryServerStatefulSet.sts, utils.ApplyPatch, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
}

// ReconcileRSExternalServerCertificate reconciles an external server certificate for the Registry Server
func (r *RegistryReconciler) ReconcileRSExternalServerCertificate(ctx context.Context, registryNamespace string, nodeName string, nodeIP string, clusterIP string, registry *metalk8sv1alpha1.Registry) error {
	nodeToken := safeNodeName(nodeName)
	registryServerServerCertificate := &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", RS_EXTERNAL_SERVER_CERTIFICATE_PREFIX, nodeToken),
			Namespace: registryNamespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, registryServerServerCertificate, func() error {
		registryServerServerCertificate.SetLabels(map[string]string{
			REG_APP_LABEL_KEY: RS_APP_LABEL_VALUE,
			NODE_LABEL_KEY:    nodeName,
		})
		err := controllerutil.SetControllerReference(registry, registryServerServerCertificate, r.Scheme)
		if err != nil {
			return err
		}
		registryServerServerCertificate.Spec.SecretName = fmt.Sprintf("%s-%s", RS_EXTERNAL_SERVER_CERTIFICATE_PREFIX, nodeToken)
		registryServerServerCertificate.Spec.IssuerRef = cmmetav1.IssuerReference{
			Name: registry.Spec.Server.CertificateIssuerRef.Name,
			Kind: registry.Spec.Server.CertificateIssuerRef.Kind,
		}
		registryServerServerCertificate.Spec.CommonName = fmt.Sprintf("%s-%s", RS_EXTERNAL_SERVER_CERTIFICATE_CN, nodeToken)
		registryServerServerCertificate.Spec.IPAddresses = []string{
			nodeIP,
			clusterIP,
		}
		registryServerServerCertificate.Spec.DNSNames = []string{
			RS_SERVICE_NAME,
			fmt.Sprintf("%s.%s", RS_SERVICE_NAME, registryNamespace),
			fmt.Sprintf("%s.%s.svc", RS_SERVICE_NAME, registryNamespace),
			fmt.Sprintf("%s.%s.svc.cluster.local", RS_SERVICE_NAME, registryNamespace),
		}
		registryServerServerCertificate.Spec.Usages = []cmv1.KeyUsage{
			cmv1.UsageKeyEncipherment,
			cmv1.UsageDigitalSignature,
			cmv1.UsageServerAuth,
		}
		return nil
	})
	return err
}

// ReconcileRSService reconciles a ClusterIP Service fronting the Registry Server
// pods. kube-proxy load-balances pulls across all replicas. It returns the
// allocated ClusterIP, which is added to the Registry Server certificate SANs.
func (r *RegistryReconciler) ReconcileRSService(ctx context.Context, registryNamespace string, registry *metalk8sv1alpha1.Registry) (string, error) {
	registryServerService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      RS_SERVICE_NAME,
			Namespace: registryNamespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, registryServerService, func() error {
		registryServerService.SetLabels(map[string]string{
			REG_APP_LABEL_KEY: RS_APP_LABEL_VALUE,
		})
		if err := controllerutil.SetControllerReference(registry, registryServerService, r.Scheme); err != nil {
			return err
		}
		registryServerService.Spec.Type = corev1.ServiceTypeClusterIP
		registryServerService.Spec.Selector = map[string]string{
			REG_APP_LABEL_KEY: RS_APP_LABEL_VALUE,
		}
		registryServerService.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "https",
				Port:       RS_SERVER_PORT,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromInt32(RS_SERVER_PORT),
			},
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if registryServerService.Spec.ClusterIP == "" {
		return "", fmt.Errorf("ClusterIP not yet allocated for Service %s", RS_SERVICE_NAME)
	}
	return registryServerService.Spec.ClusterIP, nil
}

func (cpt componentSts) setRSImageTag(registry *metalk8sv1alpha1.Registry) {
	cpt.sts.Spec.Template.Spec.Containers[0].Image = registry.Spec.Server.Image.GetImage()
}

func (cpt componentSts) setRSEnvVariables(registry *metalk8sv1alpha1.Registry, nodeIP string) {
	environmentMapping := make(map[string]int)
	for id, env := range cpt.sts.Spec.Template.Spec.Containers[0].Env {
		environmentMapping[env.Name] = id
	}

	// Change LOG_LEVEL
	logLevel := corev1.EnvVar{
		Name:  "LOG_LEVEL",
		Value: *registry.Spec.LogLevel,
	}
	if idx, exists := environmentMapping["LOG_LEVEL"]; !exists {
		cpt.sts.Spec.Template.Spec.Containers[0].Env = append(cpt.sts.Spec.Template.Spec.Containers[0].Env, logLevel)
	} else {
		cpt.sts.Spec.Template.Spec.Containers[0].Env[idx] = logLevel
	}

	// Bind the server to the node's InternalIP only
	httpAddr := corev1.EnvVar{
		Name:  "HTTP_ADDR",
		Value: fmt.Sprintf("%s:%d", nodeIP, RS_SERVER_PORT),
	}
	if idx, exists := environmentMapping["HTTP_ADDR"]; !exists {
		cpt.sts.Spec.Template.Spec.Containers[0].Env = append(cpt.sts.Spec.Template.Spec.Containers[0].Env, httpAddr)
	} else {
		cpt.sts.Spec.Template.Spec.Containers[0].Env[idx] = httpAddr
	}
}

func (cpt componentSts) setRSVolumes(registry *metalk8sv1alpha1.Registry, nodeName string) error {
	nodeToken := safeNodeName(nodeName)
	volumesMapping := make(map[string]int)
	for id, volume := range cpt.sts.Spec.Template.Spec.Volumes {
		volumesMapping[volume.Name] = id
	}

	var idVol int
	var exists bool

	// metalk8s-registry-server-solutions: where the solutions are present to be served
	idVol, exists = volumesMapping["metalk8s-registry-server-solutions"]
	if !exists {
		return fmt.Errorf("volume metalk8s-registry-server-solutions not found")
	}
	cpt.sts.Spec.Template.Spec.Volumes[idVol].VolumeSource = corev1.VolumeSource{
		HostPath: &corev1.HostPathVolumeSource{
			Path: *registry.Spec.SolutionsPath,
			Type: ptr.To(corev1.HostPathDirectory),
		},
	}

	// External Server TLS Certificate
	idVol, exists = volumesMapping[TLS_SERVER_EXTERNAL_CERTS_NAME]
	if !exists {
		return fmt.Errorf("volume %s not found", TLS_SERVER_EXTERNAL_CERTS_NAME)
	}
	cpt.sts.Spec.Template.Spec.Volumes[idVol].VolumeSource = corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{
			SecretName: fmt.Sprintf("%s-%s", RS_EXTERNAL_SERVER_CERTIFICATE_PREFIX, nodeToken),
		},
	}

	return nil
}

// getNodeInternalIP returns the node's InternalIP, or "" if none is set.
func getNodeInternalIP(node *corev1.Node) string {
	for _, address := range node.Status.Addresses {
		if address.Type == corev1.NodeInternalIP {
			return address.Address
		}
	}
	return ""
}

// getSortedNodeInternalIPs returns the sorted InternalIPs of the given nodes,
// skipping nodes without one.
func getSortedNodeInternalIPs(nodes []corev1.Node) []string {
	ips := []string{}
	for i := range nodes {
		if ip := getNodeInternalIP(&nodes[i]); ip != "" {
			ips = append(ips, ip)
		}
	}
	slices.Sort(ips)
	return ips
}

// getRegistryServerCA returns the Registry Server CA (ca.crt) and the name of
// the Secret it was read from, looking at the per-node external server
// certificate secrets. All per-node certs share the same issuer/CA.
// Returns empty strings when none is available yet.
func getRegistryServerCA(ctx context.Context, c client.Client, registryNamespace string, nodeNames []string) (string, string) {
	for _, nodeName := range nodeNames {
		secret := &corev1.Secret{}
		secretName := fmt.Sprintf("%s-%s", RS_EXTERNAL_SERVER_CERTIFICATE_PREFIX, safeNodeName(nodeName))
		if err := c.Get(ctx, types.NamespacedName{
			Name:      secretName,
			Namespace: registryNamespace,
		}, secret); err != nil {
			if !apierrors.IsNotFound(err) {
				logf.FromContext(ctx).Error(err, "failed to read registry server CA secret", "node", nodeName)
			}
			continue
		}
		if ca, ok := secret.Data["ca.crt"]; ok && len(ca) > 0 {
			return string(ca), secretName
		}
	}
	return "", ""
}

// reconcileContainerdMirrorConfigMap creates/updates (or deletes when disabled) the
// containerd mirror ConfigMap (_default/hosts.toml + ca.crt). The host list is the
// ClusterIP first, then each selected node IP sorted.
func (r *RegistryReconciler) reconcileContainerdMirrorConfigMap(ctx context.Context, registryNamespace string, registry *metalk8sv1alpha1.Registry, clusterIP string, nodes []corev1.Node) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CONTAINERD_MIRROR_CONFIGMAP_NAME,
			Namespace: registryNamespace,
		},
	}

	if !registry.IsMirrorPropagationEnabled() {
		if err := r.Delete(ctx, configMap); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("error deleting containerd mirror ConfigMap: %w", err)
		}
		return nil
	}

	mirrorHosts := []string{fmt.Sprintf("https://%s:%d", clusterIP, RS_SERVER_PORT)}
	for _, ip := range getSortedNodeInternalIPs(nodes) {
		mirrorHosts = append(mirrorHosts, fmt.Sprintf("https://%s:%d", ip, RS_SERVER_PORT))
	}

	nodeNames := make([]string, 0, len(nodes))
	for i := range nodes {
		nodeNames = append(nodeNames, nodes[i].Name)
	}
	caCrt, _ := getRegistryServerCA(ctx, r.Client, registryNamespace, nodeNames)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, configMap, func() error {
		if err := controllerutil.SetControllerReference(registry, configMap, r.Scheme); err != nil {
			return err
		}
		configMap.SetAnnotations(map[string]string{
			CERTS_D_SUBDIR_ANNOTATION: CERTS_D_SUBDIR_VALUE,
		})
		data := map[string]string{
			MIRROR_HOSTS_TOML_KEY: utils.GenerateContainerdHostsToml(mirrorHosts),
		}
		if caCrt != "" {
			data[MIRROR_CA_KEY] = caCrt
		}
		configMap.Data = data
		return nil
	})
	return err
}

// ReconcileContainerdMirrorSyncDaemonSet reconciles the DaemonSet running
// file-reflector to sync the containerd mirror ConfigMap to every node's
// containerd certs.d directory. Deletes it when mirror propagation is disabled.
// It updates the mirror sync status fields and returns the readiness of the
// mirror sync for the global readiness computation (true when disabled, so a
// disabled mirror sync does not degrade the global readiness).
func (r *RegistryReconciler) ReconcileContainerdMirrorSyncDaemonSet(ctx context.Context, registryNamespace string, registry *metalk8sv1alpha1.Registry) (bool, error) {
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CONTAINERD_MIRROR_SYNC_DAEMONSET_NAME,
			Namespace: registryNamespace,
		},
	}

	if !registry.IsMirrorPropagationEnabled() {
		if err := r.Delete(ctx, daemonSet); err != nil && !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("error deleting containerd mirror sync DaemonSet: %w", err)
		}
		registry.SetMirrorSyncAvailable(false)
		registry.SetMirrorSyncReady(false)
		return true, nil
	}

	image := registry.GetMirrorPropagationImage()

	args := []string{"--source=/source", "--target=/target"}
	if registry.Spec.MirrorPropagation != nil {
		for _, ignorePath := range registry.Spec.MirrorPropagation.IgnorePaths {
			args = append(args, fmt.Sprintf("--ignore=%s", ignorePath))
		}
	}
	args = append(args, "--file-mode=0644", "--owner=0:0")

	labels := map[string]string{
		REG_APP_LABEL_KEY: CONTAINERD_MIRROR_SYNC_DAEMONSET_NAME,
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, daemonSet, func() error {
		if err := controllerutil.SetControllerReference(registry, daemonSet, r.Scheme); err != nil {
			return err
		}
		daemonSet.SetLabels(labels)
		daemonSet.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
		daemonSet.Spec.Template.Labels = labels
		daemonSet.Spec.Template.Spec.NodeSelector = registry.GetMirrorPropagationNodeSelector()
		// Assign unconditionally so fields removed from the spec are cleared on update.
		var tolerations []corev1.Toleration
		if registry.Spec.MirrorPropagation != nil {
			tolerations = registry.Spec.MirrorPropagation.Tolerations
		}
		daemonSet.Spec.Template.Spec.Tolerations = tolerations
		daemonSet.Spec.Template.Spec.ImagePullSecrets = image.PullSecrets

		container := corev1.Container{
			Name:  "containerd-mirror-sync",
			Image: image.GetImage(),
			Args:  args,
			SecurityContext: &corev1.SecurityContext{
				RunAsNonRoot:             ptr.To(true),
				ReadOnlyRootFilesystem:   ptr.To(true),
				AllowPrivilegeEscalation: ptr.To(false),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
					// Required to write root:root/0644 files on the host while
					// running as non-root (file caps are set on the binary).
					Add: []corev1.Capability{"DAC_OVERRIDE", "FOWNER", "CHOWN"},
				},
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("10m"),
					corev1.ResourceMemory: resource.MustParse("16Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("32Mi"),
				},
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "mirror-config",
					MountPath: "/source",
					ReadOnly:  true,
				},
				{
					Name:      "host-certs-d",
					MountPath: "/target",
				},
			},
		}
		if image.PullPolicy != nil {
			container.ImagePullPolicy = *image.PullPolicy
		}
		daemonSet.Spec.Template.Spec.Containers = []corev1.Container{container}

		daemonSet.Spec.Template.Spec.Volumes = []corev1.Volume{
			{
				Name: "mirror-config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: CONTAINERD_MIRROR_CONFIGMAP_NAME,
						},
					},
				},
			},
			{
				Name: "host-certs-d",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						// Only the _default subdir is managed by the reflector: the
						// rest of the certs.d directory is left untouched.
						Path: fmt.Sprintf("%s/%s", registry.GetContainerdConfigPath(), CERTS_D_SUBDIR_VALUE),
						Type: ptr.To(corev1.HostPathDirectoryOrCreate),
					},
				},
			},
		}
		return nil
	})
	if err != nil {
		return false, err
	}

	registry.SetMirrorSyncAvailable(true)
	ready := daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled
	registry.SetMirrorSyncReady(ready)
	return ready, nil
}
