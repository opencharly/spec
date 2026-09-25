// CUE schema for the `kindcluster` kind. #Kindcluster validates ONE value of the
// `kindcluster:` map — a local Kubernetes-in-Docker cluster provisioned by the
// upstream `kind` tool (kubernetes-sigs/kind), the local-cluster sibling of the
// `kubernetes:` cluster template. CLOSED — every Kindcluster field is modeled, so
// an unknown key is a typo. Shared #Step from _common.cue; the cluster-policy
// sub-blocks (storage/ingress/image_default/pod_default/defaults) are the SAME
// defs the `kubernetes:` template uses (R3 — one policy surface for both).
//
// The word is `kindcluster`, NOT `kind`: the bare word collides with this schema's
// own `source: {kind: …}` discriminator and with kind's own `kind: Cluster` config
// field.
//
// Template vs deploy: like every substrate kind, the value is the disjunction
// `#Kindcluster | #DeployValue` (see node.cue #KindclusterValue), routed by SHAPE in
// the loader — a TEMPLATE carries the cluster config (engine/node_image/nodes); a
// DEPLOY carries `from:`/`image:` + the deploy config (`engine:`, `deploy:`). The
// workload the cluster runs reuses #KubernetesDeploy (#Deploy.deploy:) unchanged —
// the same Kustomize tree the `kubernetes:` substrate generates.

#Kindcluster: {
	// May be empty (a cluster-policy-only template runs no workload itself).
	box: string @go(Box)

	// engine selects the container engine kind provisions its node containers with.
	// It maps 1:1 to kind's KIND_EXPERIMENTAL_PROVIDER (podman/docker/nerdctl), so
	// the authored #EngineName vocabulary is used directly — no separate selector.
	// Absent → the resolved runtime engine (engine.run).
	engine?: #EngineName @go(Engine)

	// node_image is the digest-pinned kind node image (kindest/node:vX@sha256:…).
	// Empty → the plugin's pinned default (the kind release's default node image).
	// A digest pin is required for reproducibility (kind's own guidance).
	node_image?: string @go(NodeImage)

	// nodes is the cluster topology. Absent → a single control-plane node.
	nodes?: [...#KindclusterNode] @go(Nodes)

	// kubeconfig_context is the kubeconfig context this cluster deploys to. kind
	// writes the context named after the cluster; a deploy may override it.
	kubeconfig_context?: string @go(KubeconfigContext)

	// default_namespace mirrors the kubernetes template's default namespace.
	default_namespace?: *"default" | (string & =~"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$") @go(DefaultNamespace)

	// admission_policy mirrors the kubernetes template's admission policy (read by
	// candy/plugin-k8sgen to set the pod security context).
	admission_policy?: "restricted" | "baseline" | "privileged" @go(AdmissionPolicy)

	// Cluster policy — the SAME defs the `kubernetes:` template uses (R3).
	storage?:       #KubernetesStorage         @go(Storage,optional=nillable)
	ingress?:       #KubernetesIngressDefaults @go(Ingress,optional=nillable)
	image_default?: #KubernetesImagesDefaults  @go(ImageDefault,optional=nillable)
	pod_default?:   #KubernetesPodDefaults     @go(PodDefault,optional=nillable)
	defaults?:      #KubernetesResourceDefaults @go(Defaults)

	plan?: [...#Step] @go(Plan)
}

// #KindclusterNode — one node in the cluster topology. role discriminates the
// control plane from workers; kind names control-plane nodes `<cluster>-control-plane`
// and workers `<cluster>-worker`, `<cluster>-worker2`, …
#KindclusterNode: {
	role: *"worker" | "control-plane" @go(Role)
	// image overrides the cluster-level node_image for this node.
	image?: string @go(Image)
	// extra_port_mappings maps node ports to host ports (kind extraPortMappings) —
	// the ingress/NodePort reach path. Bind a high host port to avoid needing
	// net.ipv4.ip_unprivileged_port_start lowered for 80/443.
	extra_port_mappings?: [...#KindclusterPortMapping] @go(ExtraPortMappings)
}

#KindclusterPortMapping: {
	container_port: int & >0 & <=65535 @go(ContainerPort,type=int)
	host_port:      int & >0 & <=65535 @go(HostPort,type=int)
	listen_address: *"0.0.0.0" | string   @go(ListenAddress)
	protocol:       *"TCP" | "UDP"        @go(Protocol)
}
