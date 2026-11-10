package schema

// CUE schema for the `kubernetes` kind. #Kubernetes validates ONE value of the
// `kubernetes:` map (KubernetesSpec; absorbed the former ClusterProfile). CLOSED —
// every KubernetesSpec field is
// modeled, so an unknown key is a typo. The documented enum domains + sub-object
// shapes are constrained. ONE exception: `pod_default.tolerations` stays OPEN —
// it is a genuine passthrough of raw Kubernetes Toleration objects
// ([]map[string]any). Plural field names that mirror Kubernetes output keys are
// preserved verbatim. Shared #Step from 00_common.cue.

#Kubernetes: {
	// May be empty (a cluster-policy-only template runs no workload itself).
	box: string

	kubeconfig_context?: string                                                      @go(KubeconfigContext)
	// kubeconfig — the kubeconfig FILE this cluster is reached through. An
	// externally-managed cluster (OVH / GKE / EKS / on-prem) is usually addressed
	// by a per-cluster file rather than by the ambient `$KUBECONFIG` /
	// `~/.kube/config`; without this field the only handle a deploy has is a
	// context NAME, which cannot be resolved against a file charly was never
	// told about. Empty keeps the ambient file that plain `kubectl` reads — what
	// a charly-created k3s/kind cluster relies on.
	kubeconfig?: string @go(Kubeconfig)
	admission_policy?:   or(_kubernetesAdmissionPolicyWord)                    @go(AdmissionPolicy)
	default_namespace?:  *"default" | (string & =~"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$") @go(DefaultNamespace)

	storage?:        #KubernetesStorage
	ingress?:        #KubernetesIngressDefaults
	image_default?:  #KubernetesImagesDefaults @go(ImageDefault)
	pod_default?:    #KubernetesPodDefaults    @go(PodDefault)
	// kubevirt — cluster-specific KubeVirt knobs (the kind:kubevirt substrate's
	// cluster side). Mirrors the storage/ingress "one cluster template owns the
	// cluster-specific policy" partition: a kind:kubevirt VM names its cluster by
	// `cluster:` and these knobs apply.
	kubevirt?: #KubernetesKubeVirt @go(KubeVirt)
	defaults?: #KubernetesResourceDefaults
}

// #KubernetesKubeVirt — the KubeVirt platform knobs a cluster template carries.
// The kubevirt-operator install + the kind:kubevirt substrate's defaults resolve
// from here.
#KubernetesKubeVirt: {
	// enabled — whether this cluster has KubeVirt available (informational;
	// the operator candy owns the actual install).
	enabled?: bool
	// cdi_enabled — whether CDI is installed (needed for a data_volume boot).
	cdi_enabled?: bool @go(CDIEnabled)
	cdi_namespace?: string @go(CDINamespace)
	// use_emulation runs KubeVirt's software emulation (no /dev/kvm on the node).
	// The R10 bed uses REAL KVM, so this defaults false.
	use_emulation?: bool @go(UseEmulation)
	default_storage_class?: string @go(DefaultStorageClass)
	default_instancetype?:  string @go(DefaultInstancetype)
	default_preference?:    string @go(DefaultPreference)
	// snapshot_class names the VolumeSnapshotClass a VirtualMachineSnapshot uses.
	snapshot_class?: string @go(SnapshotClass)
	// virtctl_version pins the virtctl CLI release the operator candy installs.
	virtctl_version?: string @go(VirtctlVersion)
	feature_gates?: [...string] @go(FeatureGates)
}

#KubernetesStorage: {
	class_default?:       string @go(ClassDefault)
	class_cheap?:         string @go(ClassCheap)
	class_encrypted?:     string @go(ClassEncrypted)
	class_fast?:          string @go(ClassFast)
	access_mode_default?: or(_accessModeDefaultWord) @go(AccessModeDefault)
}
#KubernetesIngressDefaults: {
	enabled?:           bool
	class?:             string
	cert_issuer?:       string                                                 @go(CertIssuer)
	path_type_default?: or(_pathTypeDefaultWord) @go(PathTypeDefault)
}
#KubernetesImagesDefaults: {
	pull_policy?: or(_kubernetesImagesDefaultsPullPolicyWord) @go(PullPolicy)
	pull_secrets?: [...string] @go(PullSecrets)
}
#KubernetesPodDefaults: {
	priority_class?: string @go(PriorityClass)
	// Raw Kubernetes Toleration objects (Go []map[string]any) — genuine
	// passthrough, so each element stays OPEN.
	tolerations?: [...{...}]
	node_selector?: {[string]: string} @go(NodeSelector)
}
#KubernetesResourceDefaults: {
	labels?: [string]:      string
	annotations?: [string]: string
}

// --- vocabulary tables (S3) --------------------------------------------------
// Every closed set of words is a table keyed by word, `<x>Word` is derived from it,
// and the enum an author meets is `or(<x>Word)` — so the table and the enum cannot
// disagree and a new word is ONE row here. `@vocab` names the owner (§3.1): these
// sets are compiled into THIS contract module, so the owner is `spec`.
//
// Each table is named after its word when that name is unique in the schema, and
// `<definition><Field>` when it is not; the comment above it names its site.

// #Kubernetes.admission_policy
_kubernetesAdmissionPolicy: {restricted: {}, baseline: {}, privileged: {}} @vocab(spec)
_kubernetesAdmissionPolicyWord: [for w, _ in _kubernetesAdmissionPolicy {w}]
// #KubernetesStorage.access_mode_default
_accessModeDefault: {
	"ReadWriteOnce": {}
	"ReadWriteMany": {}
	"ReadOnlyMany": {}
	"ReadWriteOncePod": {}
} @vocab(spec)
_accessModeDefaultWord: [for w, _ in _accessModeDefault {w}]
// #KubernetesIngressDefaults.path_type_default
_pathTypeDefault: {"Prefix": {}, "Exact": {}, "ImplementationSpecific": {}} @vocab(spec)
_pathTypeDefaultWord: [for w, _ in _pathTypeDefault {w}]
// #KubernetesImagesDefaults.pull_policy
_kubernetesImagesDefaultsPullPolicy: {
	"IfNotPresent": {}
	"Always": {}
	"Never": {}
} @vocab(spec)
_kubernetesImagesDefaultsPullPolicyWord: [for w, _ in _kubernetesImagesDefaultsPullPolicy {w}]
