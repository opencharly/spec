// Package checkstep hosts the TYPED-STEP state-provision contract cluster for host-coupled
// check-verb candies (#55 CHECK-ENGINE cone Option A): the OPTIONAL StepProvider /
// ProvisionActor roles a kit candy implements alongside CheckVerbProvider, and the
// ResolvePackageName cross-distro resolver.
//
// The StepProvider role lets the CANDY own BOTH halves of its typed install-step lowering:
// StepKind names the internal InstallPlan IR spec.StepKind it lowers into, and MaterializeStep
// builds the real spec.InstallStep from the op's plugin_input + the host-resolved ctx scalars
// (run-as user, candy name, package format, distro tags). Core holds NO per-kind switch — its
// in-proc kit adapter (charly/check_kit_adapter.go) delegates both calls to the provider, since
// a core kind switch is an incomplete seam (the kernel/plugin boundary law).
//
// Housing this contract here (not spec/spec) lets charly core's in-proc kitVerbAdapter
// reference it importing zero kit, while sdk/kit re-exports each symbol
// (sdk/kit/check_step_descriptors.go) so every candy call site compiles UNCHANGED.
// CheckVerbProvider / CheckContext / the CheckContext scalar types stay in spec/spec
// (checkcontext.go) — this cluster is the step-role sibling, imported here alongside
// spec/spec for the *spec.Op the StepProvider / ProvisionActor methods take.
package checkstep

import "github.com/opencharly/spec/spec"

// ResolvePackageName picks the correct package name for the running image's distro: if
// packageMap has a key matching any of the image's distro tags (first match wins — tags
// are authored most-specific-first, "fedora:43" before "fedora"), that mapping is used;
// otherwise the bare pkg name. The single cross-distro name resolver shared by the
// `package` candy's check + act + step materializer (R3).
func ResolvePackageName(pkg string, packageMap map[string]string, distros []string) string {
	if len(packageMap) == 0 {
		return pkg
	}
	for _, tag := range distros {
		if name, ok := packageMap[tag]; ok && name != "" {
			return name
		}
	}
	return pkg
}

// StepProvider is the OPTIONAL third role of a host-coupled verb candy: a verb whose
// build/deploy ACT lowers into a TYPED install-plan step (service → ServicePackagedStep,
// package → SystemPackagesStep) rather than a shell (ProvisionActor) or a generic OpStep.
// The CANDY owns the whole lowering: StepKind names the target internal IR spec.StepKind
// (the static half), and MaterializeStep builds the real spec.InstallStep for one op (the
// dynamic half). The host's kit adapter (charly/check_kit_adapter.go) delegates both calls
// to the provider, so core keeps NO per-kind mapping and NO materializer switch — the
// kernel/plugin boundary law's incomplete-seam tell is exactly a per-kind branch in core.
//
// op is the verb's *spec.Op (the verb's plugin_input rides op.PluginInput). The four ctx
// scalars are host-RESOLVED before the call: runAsUser is the resolved user directive
// (deploykit.ResolveUserSpec's result), candyName the layer's name, pkgFormat the image's
// package format, and distroTags the image's distro tag list (most-specific-first, the input
// ResolvePackageName consumes). The load-bearing Reverse()s stay on the returned step
// (package main owns the reversal timeline); the candy owns only the construction.
type StepProvider interface {
	StepKind() spec.StepKind
	MaterializeStep(op *spec.Op, runAsUser, candyName, pkgFormat string, distroTags []string) spec.InstallStep
}

// ProvisionActor is the OPTIONAL second role of a host-coupled verb candy: the do:act
// renderer for a state-provision verb (kernel_param/mount/user/unix_group/file/command/
// service/package), rendering the shell that ENACTS the op under the live init / package
// manager. It is reached at install COMPILE+EMIT (a `run: {plugin: <verb>}` step → the
// build-act RUN in emitTasks, and the local/vm deploy act) AND at runtime act. A candy
// whose verb type implements this ALONGSIDE CheckVerbProvider is registered as a
// multi-role provider (the host adapter then also satisfies the package-main
// ProvisionActor). op is the *spec.Op (the verb's plugin_input rides op.PluginInput);
// distros is the image's distro tag list for package-name resolution. Returns
// (script, ok); ok=false means "no act form for this op" (the host skips/errors per its
// act path). This is the SHELL-string act role — a verb that instead lowers into a typed
// InstallPlan step (service/package) additionally needs the StepProvider contract.
type ProvisionActor interface {
	Reserved() string
	RenderProvisionScript(op *spec.Op, distros []string) (script string, ok bool)
}
