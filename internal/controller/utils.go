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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	ORGANIZATION_NAME                      = "metalk8s"
	SSA_FIELD_OWNER_NAME                   = "registry-operator"
	RNA_CA_NAME                            = "metalk8s-registry-node-agent-ca"
	RNA_CA_SECRET_NAME                     = "rna-ca-cert"
	RNA_CA_ISSUER_NAME                     = "metalk8s-registry-node-agent-ca-issuer"
	RNA_SELFSIGNED_ISSUER_NAME             = "metalk8s-registry-node-agent-selfsigned-issuer"
	RNA_SELFSIGNED_ISSUER_KIND             = "Issuer"
	RNA_STATEFULSET_PREFIX                 = "metalk8s-registry-node-agent"
	RNA_INTERNAL_SERVER_CERTIFICATE_PREFIX = "rna-internal-server"
	RNA_INTERNAL_SERVER_CERTIFICATE_CN     = "rna-internal-server"
	RNA_EXTERNAL_SERVER_CERTIFICATE_PREFIX = "rna-external-server"
	RNA_INTERNAL_CLIENT_CERTIFICATE_PREFIX = "rna-internal-client"
	RNA_EXTERNAL_CLIENT_CERTIFICATE_PREFIX = "rna-external-client"
	TLS_CLIENT_INTERNAL_CERTS_NAME         = "tls-client-intern-certs"
	TLS_SERVER_INTERNAL_CERTS_NAME         = "tls-server-intern-certs"
	TLS_SERVER_EXTERNAL_CERTS_NAME         = "tls-server-extern-certs"
	TLS_CLIENT_EXTERNAL_CERTS_NAME         = "tls-client-extern-certs"
	RNA_FINALIZER_NAME                     = "metalk8s.scality.com/finalizer"
	RNA_DEFAULT_NAMESPACE                  = "metalk8s-registry"
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

// ReconcileRNAExternalClientCACertificate reconciles a namespaced copy of the CA certificate defined in the Registry (spec.agent.authentication.mtls.caSecretRef)
// It is used to sign the mTLS Certificate used to upload ISO files
func (r *RegistryReconciler) ReconcileRNAExternalClientCACertificate(ctx context.Context, registryNamespace string, registry *metalk8sv1alpha1.Registry) error {
	caSecretRef := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      registry.Spec.Agent.Authentication.MTLS.CASecretRef.Name,
		Namespace: registry.Spec.Agent.Authentication.MTLS.CASecretRef.Namespace,
	}, caSecretRef); err != nil {
		return err
	}

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

/*
	Beyond this point, the functions are specific to one Registry Node Agent instance on a Node:
*/

type rnaSts struct {
	sts *appsv1.StatefulSet
}

// ReconcileRNAStatefulSet reconciles a Registry Node Agent as a StatefulSet on the specified node
func (r *RegistryReconciler) ReconcileRNAStatefulSet(ctx context.Context, registryNamespace string, nodeName string, registry *metalk8sv1alpha1.Registry) error {
	registryNodeAgentStatefulSet := rnaSts{r.RNA.StatefulSets[0].DeepCopy()}

	// Check for existing StatefulSet on the node
	registryNodeAgentStatefulSets := &appsv1.StatefulSetList{}
	err := r.List(ctx, registryNodeAgentStatefulSets,
		client.InNamespace(registryNamespace),
		client.MatchingLabels(map[string]string{RNA_APP_LABEL_KEY: RNA_APP_LABEL_VALUE, "node": nodeName}),
	)
	if err != nil {
		return err
	}
	if len(registryNodeAgentStatefulSets.Items) > 0 {
		sts := &registryNodeAgentStatefulSets.Items[0]
		utils.CleanResource(sts)
		registryNodeAgentStatefulSet = rnaSts{sts}
	}

	// Set metadata on StatefulSet
	registryNodeAgentStatefulSet.sts.SetName(fmt.Sprintf("%s-%s", RNA_STATEFULSET_PREFIX, nodeName))
	registryNodeAgentStatefulSet.sts.SetNamespace(registryNamespace)
	registryNodeAgentStatefulSet.sts.Labels["node"] = nodeName
	if err := controllerutil.SetControllerReference(registry, registryNodeAgentStatefulSet.sts, r.Scheme); err != nil {
		return err
	}

	// Modify replica to 1
	registryNodeAgentStatefulSet.sts.Spec.Replicas = ptr.To(int32(1))

	// Modify Security Context
	registryNodeAgentStatefulSet.setPodSecurityContext()

	// Set affinity to the specified node to ensure the StatefulSet is scheduled on the specified node
	registryNodeAgentStatefulSet.setAffinity(nodeName)

	// Set Registry/Image:Tag
	registryNodeAgentStatefulSet.setRegistryImageTag(registry)

	// Set Node label on Pod
	registryNodeAgentStatefulSet.setNodeLabel(nodeName)

	// Set Volumes (archives, solutions, dev, TLS/mTLS Certificates, webhook-certs)
	if err := registryNodeAgentStatefulSet.setVolumes(registry, nodeName); err != nil {
		return err
	}

	// Set Environment Variables (NODE_IP, LOGLEVEL)
	registryNodeAgentStatefulSet.setEnvVariables(nodeName, registryNamespace, registry)

	// examine DeletionTimestamp to determine if object is under deletion
	if registryNodeAgentStatefulSet.sts.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then let's add the finalizer and update the object. This is equivalent
		// to registering our finalizer.
		if !controllerutil.ContainsFinalizer(registryNodeAgentStatefulSet.sts, RNA_FINALIZER_NAME) {
			controllerutil.AddFinalizer(registryNodeAgentStatefulSet.sts, RNA_FINALIZER_NAME)
		}
	}

	return r.Patch(ctx, registryNodeAgentStatefulSet.sts, client.Apply, client.ForceOwnership, client.FieldOwner(SSA_FIELD_OWNER_NAME))
}

// ReconcileRNAService reconciles a Registry Node Agent service
func (r *RegistryReconciler) ReconcileRNAService(ctx context.Context, registryNamespace string, nodeName string, registry *metalk8sv1alpha1.Registry) error {
	registryNodeAgentService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", RNA_INTERNAL_SERVER_CERTIFICATE_CN, nodeName),
			Namespace: registryNamespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, registryNodeAgentService, func() error {
		registryNodeAgentService.SetLabels(map[string]string{
			RNA_APP_LABEL_KEY: RNA_APP_LABEL_VALUE,
			"node":            nodeName,
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
			RNA_APP_LABEL_KEY: RNA_APP_LABEL_VALUE,
			"control-plane":   "controller-manager",
			"node":            nodeName,
		}
		registryNodeAgentService.Spec.Type = corev1.ServiceTypeClusterIP

		return nil
	})
	return err
}

func (rna rnaSts) setAffinity(nodeName string) {
	rna.sts.Spec.Template.Spec.Affinity = &corev1.Affinity{
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

func (rna rnaSts) setRegistryImageTag(registry *metalk8sv1alpha1.Registry) {
	rna.sts.Spec.Template.Spec.Containers[0].Image = registry.Spec.Agent.Image.GetImage()
}

func (rna rnaSts) setNodeLabel(nodeName string) {
	rna.sts.Spec.Template.Labels["node"] = nodeName
}

func (rna rnaSts) setVolumes(registry *metalk8sv1alpha1.Registry, nodeName string) error {
	volumesMapping := make(map[string]int)
	for id, volume := range rna.sts.Spec.Template.Spec.Volumes {
		volumesMapping[volume.Name] = id
	}

	var idVol int
	var exists bool

	// metalk8s-registry-node-agent-archives: where the ISO files are stored
	idVol, exists = volumesMapping["metalk8s-registry-node-agent-archives"]
	if !exists {
		return fmt.Errorf("volume metalk8s-registry-node-agent-archives not found")
	}
	rna.sts.Spec.Template.Spec.Volumes[idVol].VolumeSource = corev1.VolumeSource{
		HostPath: &corev1.HostPathVolumeSource{
			Path: *registry.Spec.ArchivesPath,
			Type: ptr.To(corev1.HostPathDirectory),
		},
	}

	// metalk8s-registry-node-agent-solutions: where the solutions are mounted
	idVol, exists = volumesMapping["metalk8s-registry-node-agent-solutions"]
	if !exists {
		return fmt.Errorf("volume metalk8s-registry-node-agent-solutions not found")
	}
	rna.sts.Spec.Template.Spec.Volumes[idVol].VolumeSource = corev1.VolumeSource{
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
	rna.sts.Spec.Template.Spec.Volumes[idVol].VolumeSource = corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{
			SecretName: fmt.Sprintf("%s-%s", RNA_EXTERNAL_SERVER_CERTIFICATE_PREFIX, nodeName),
		},
	}

	// External Client CA mTLS Certificate
	idVol, exists = volumesMapping[TLS_CLIENT_EXTERNAL_CERTS_NAME]
	if !exists {
		return fmt.Errorf("volume %s not found", TLS_CLIENT_EXTERNAL_CERTS_NAME)
	}
	rna.sts.Spec.Template.Spec.Volumes[idVol].VolumeSource = corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{
			SecretName: RNA_EXTERNAL_CLIENT_CERTIFICATE_PREFIX,
		},
	}

	// Internal Server TLS Certificate
	idVol, exists = volumesMapping[TLS_SERVER_INTERNAL_CERTS_NAME]
	if !exists {
		return fmt.Errorf("volume %s not found", TLS_SERVER_INTERNAL_CERTS_NAME)
	}
	rna.sts.Spec.Template.Spec.Volumes[idVol].VolumeSource = corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{
			SecretName: fmt.Sprintf("%s-%s", RNA_INTERNAL_SERVER_CERTIFICATE_PREFIX, nodeName),
		},
	}

	// Internal Client mTLS Certificate
	idVol, exists = volumesMapping[TLS_CLIENT_INTERNAL_CERTS_NAME]
	if !exists {
		return fmt.Errorf("volume %s not found", TLS_CLIENT_INTERNAL_CERTS_NAME)
	}
	rna.sts.Spec.Template.Spec.Volumes[idVol].VolumeSource = corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{
			SecretName: fmt.Sprintf("%s-%s", RNA_INTERNAL_CLIENT_CERTIFICATE_PREFIX, nodeName),
		},
	}

	return nil
}

func (rna rnaSts) setEnvVariables(nodeName string, registryNamespace string, registry *metalk8sv1alpha1.Registry) {
	environmentMapping := make(map[string]int)
	for id, env := range rna.sts.Spec.Template.Spec.Containers[0].Env {
		environmentMapping[env.Name] = id
	}

	// Change NODE_IP
	nodeIP := corev1.EnvVar{
		Name:  "NODE_IP",
		Value: fmt.Sprintf("%s-%s.%s.svc", RNA_INTERNAL_SERVER_CERTIFICATE_CN, nodeName, registryNamespace),
	}
	if idx, exists := environmentMapping["NODE_IP"]; !exists {
		rna.sts.Spec.Template.Spec.Containers[0].Env = append(rna.sts.Spec.Template.Spec.Containers[0].Env, nodeIP)
	} else {
		rna.sts.Spec.Template.Spec.Containers[0].Env[idx] = nodeIP
	}

	// Change LOGGER_LOG_LEVEL
	logLevel := corev1.EnvVar{
		Name:  "LOGGER_LOG_LEVEL",
		Value: *registry.Spec.LogLevel,
	}
	if idx, exists := environmentMapping["LOGGER_LOG_LEVEL"]; !exists {
		rna.sts.Spec.Template.Spec.Containers[0].Env = append(rna.sts.Spec.Template.Spec.Containers[0].Env, logLevel)
	} else {
		rna.sts.Spec.Template.Spec.Containers[0].Env[idx] = logLevel
	}
}

func (rna rnaSts) setPodSecurityContext() {
	rna.sts.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
		FSGroup:      ptr.To(int64(0)),
		RunAsGroup:   ptr.To(int64(0)),
		RunAsNonRoot: ptr.To(false),
		RunAsUser:    ptr.To(int64(0)),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
		SupplementalGroups: []int64{6},
	}
}
