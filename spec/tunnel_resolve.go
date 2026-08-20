package spec

import (
	"fmt"
	"os"
	"strconv"
)

// tunnel_resolve.go — the pure tunnel-config RESOLUTION value-transform, promoted from
// sdk/deploykit (#55 U5 value-type consolidation, unblocked by U3's spec.ParsePortMapping).
// TunnelConfigFromMetadata turns image-label metadata into a ready-to-execute TunnelConfig;
// parseHostPorts / buildPortMapping / resolveProto are its pure helpers. Every dependency is a
// spec value type + spec.ParsePortMapping — no *Config/*Candy live graph, no host I/O beyond a
// stderr diagnostic — so it is a spec-hosted contract transform an import-clean charly file /
// plugin reaches without an sdk mechanism-kit import. sdk/deploykit keeps a re-export forwarder
// so its callers compile unchanged. This also RETIRED the dead FUNCTIONAL duplicate in
// sdk/kit/tunnel_metadata.go — a stale copy of these helpers (zero production callers), R3.

// parseHostPorts extracts host-side ports from image port mappings via the canonical
// ParsePortMapping. Unparseable entries are reported on stderr — silent skipping was the root
// cause of an unrelated bug where tunnel rules vanished without a diagnostic.
func parseHostPorts(boxPorts []string) []int {
	var result []int
	for _, mapping := range boxPorts {
		p, ok := ParsePortMapping(mapping)
		if !ok {
			fmt.Fprintf(os.Stderr,
				"Warning: ignoring unparseable port mapping %q (expected forms: \"P\", \"H:C\", \"IP:H:C\")\n",
				mapping)
			continue
		}
		result = append(result, p.Host)
	}
	return result
}

// buildPortMapping builds a host→container port map from image port mappings.
// Same loud-failure policy as parseHostPorts — see comment above.
func buildPortMapping(boxPorts []string) map[int]int {
	m := make(map[int]int, len(boxPorts))
	for _, mapping := range boxPorts {
		p, ok := ParsePortMapping(mapping)
		if !ok {
			fmt.Fprintf(os.Stderr,
				"Warning: ignoring unparseable port mapping %q (expected forms: \"P\", \"H:C\", \"IP:H:C\")\n",
				mapping)
			continue
		}
		m[p.Host] = p.Container
	}
	return m
}

// resolveProto returns the backend scheme for a container port, defaulting to "http".
// portProtos is string-keyed (the OCI-label wire form, P2B reshape) — index by the port as a string.
func resolveProto(containerPort int, portProtos map[string]string) string {
	if portProtos != nil {
		if pp, ok := portProtos[strconv.Itoa(containerPort)]; ok {
			return pp
		}
	}
	return "http"
}

// TunnelConfigFromMetadata creates a TunnelConfig from image-label metadata. It is the
// SINGLE tunnel-resolution entry point — the former TunnelYAML-based resolution function
// (with its unused CandyReader/[]string DI params) was deleted (R5): its only production
// caller, the deploy-overlay `charly box inspect` tunnel display, resolves the same shape by
// constructing a BoxMetadata from the overlay's Tunnel + published-port set.
func TunnelConfigFromMetadata(meta *BoxMetadata) *TunnelConfig {
	if meta == nil || meta.Tunnel == nil {
		return nil
	}

	t := meta.Tunnel
	cfg := &TunnelConfig{
		Provider: t.Provider,
		BoxName:  meta.Box,
	}

	hostPorts := parseHostPorts(meta.Port)
	hostToContainer := buildPortMapping(meta.Port)

	// Determine public set
	publicSet := make(map[int]bool)
	publicHostnames := make(map[int]string)
	if t.Public.All {
		for _, p := range hostPorts {
			publicSet[p] = true
		}
	}
	for _, p := range t.Public.Ports {
		publicSet[p] = true
	}
	for p, h := range t.Public.PortMap {
		publicSet[p] = true
		publicHostnames[p] = h
	}

	// Determine private set
	privateSet := make(map[int]bool)
	if t.Private.All {
		for _, p := range hostPorts {
			if !publicSet[p] {
				privateSet[p] = true
			}
		}
	}
	for _, p := range t.Private.Ports {
		privateSet[p] = true
	}

	// Build TunnelPort slice
	for _, hp := range hostPorts {
		if !publicSet[hp] && !privateSet[hp] {
			continue
		}
		cp := hp
		if c, ok := hostToContainer[hp]; ok {
			cp = c
		}
		proto := resolveProto(cp, meta.PortProto)
		cfg.Ports = append(cfg.Ports, TunnelPort{
			Port:        hp,
			BackendPort: hp,
			Protocol:    proto,
			Public:      publicSet[hp],
			Hostname:    publicHostnames[hp],
		})
	}

	// Cloudflare defaults
	if cfg.Provider == "cloudflare" {
		cfg.TunnelName = t.Tunnel
		if cfg.TunnelName == "" {
			cfg.TunnelName = "charly-" + meta.Box
		}
		cfg.Hostname = meta.DNS
	}

	return cfg
}
