package container

// image_inspect.go — the container→image-ref + image-label inspectors, RELOCATED to the
// spec/container fabric slice (#55 CHECK-ENGINE cone Option A — pure podman/docker CLI
// invocations importing zero kit). The ONE container→image-ref inspector (ContainerImageRef) + the
// image-label reader (InspectImageLabels), shared between charly core's remaining callers
// (start.go, commands.go, service.go) and the candies — the former core caller,
// check_endpoint_resolve.go's resolveImageLabelFor, relocated to candy/plugin-check's
// resolve_endpoint.go (#55 W3 B7); it calls THESE functions directly now, same as the other
// candies. sdk/kit re-exports each (sdk/kit/container_image.go + sdk/kit/local_image.go) so every
// existing kit.ContainerImageRef / kit.ContainerImage / kit.InspectImageLabels call site is
// untouched. New consumers reference spec/container directly.

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/opencharly/spec/cache"
	execc "github.com/opencharly/spec/exec"
	"github.com/opencharly/spec/spec"
)

// ContainerImageRef returns the image ref backing a running container (.Config.Image via
// `<engine> inspect`). THE single container→image-ref inspector — there is exactly one inspect
// implementation.
func ContainerImageRef(engine, containerName string) (string, error) {
	out, _, exit, err := execc.RunCaptureCmd(exec.Command(EngineBinary(engine), "inspect", "--format", "{{.Config.Image}}", containerName))
	if err != nil {
		return "", fmt.Errorf("inspecting container %s: %w", containerName, err)
	}
	if exit != 0 {
		return "", fmt.Errorf("inspect %s: exit %d", containerName, exit)
	}
	return strings.TrimSpace(out), nil
}

// ContainerImage returns the image ref for a running container, best-effort ("" on error). Thin
// wrapper over ContainerImageRef.
func ContainerImage(engine, containerName string) string {
	ref, _ := ContainerImageRef(engine, containerName)
	return ref
}

// InspectImageLabels reads a local image's OCI labels via engine inspect. Pure container-storage
// probe: no charly-core coupling. CACHED persistently and content-addressed: the cache key is the
// engine + the image ref (a rebuilt image has a new ref), so a rebuild is an immediate miss and an
// unchanged image is served however old the entry is — no TTL. The LIVE container state (podman
// ps) is never cached.
func InspectImageLabels(engine, imageRef string) (map[string]string, error) {
	cachePath, key := imageLabelsCacheKey(engine, imageRef)
	if cachePath != "" {
		if labels, ok := readImageLabelsCache(cachePath, key); ok {
			return labels, nil
		}
	}
	labels, err := inspectImageLabelsUncached(engine, imageRef)
	if err != nil {
		return nil, err
	}
	if cachePath != "" {
		writeImageLabelsCache(cachePath, key, labels)
	}
	return labels, nil
}

func inspectImageLabelsUncached(engine, imageRef string) (map[string]string, error) {
	binary := EngineBinary(engine)
	cmd := exec.Command(binary, "inspect", "--format", "{{json .Config.Labels}}", imageRef)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("inspecting %s: %w", imageRef, err)
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "null" || trimmed == "" {
		return nil, nil
	}

	var labels map[string]string
	if err := json.Unmarshal([]byte(trimmed), &labels); err != nil {
		return nil, fmt.Errorf("parsing labels from %s: %w", imageRef, err)
	}
	return labels, nil
}

// imageLabelsCacheKey returns the image-label cache file + the content key. The
// labels are a function of the image's CONTENT, so the key is the engine + the
// ref + the STORE GENERATION (imageStoreFingerprint). The ref alone is NOT
// sufficient: a mutable tag (":latest") names different content after a rebuild,
// and only the store generation changes then — so a rebuild is an immediate miss
// and an unchanged store is served however old the entry is. No TTL.
func imageLabelsCacheKey(engine, imageRef string) (string, string) {
	cfg, err := spec.DefaultDeployConfigPath()
	if err != nil {
		return "", ""
	}
	return filepath.Join(filepath.Dir(cfg), "cache", "labels.json"),
		cache.Key("labels", engine, imageRef, imageStoreFingerprint(engine))
}

// readImageLabelsCache returns the cached labels for key, else (nil, false). A
// corrupt/absent file is a cache miss.
func readImageLabelsCache(path, key string) (map[string]string, bool) {
	var labels map[string]string
	if !cache.Read(path, key, &labels) {
		return nil, false
	}
	return labels, true
}

// writeImageLabelsCache persists the labels (best-effort).
func writeImageLabelsCache(path, key string, labels map[string]string) {
	cache.Write(path, key, labels)
}
