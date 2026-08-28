package spec

import "testing"

// DistroOvmfFamilies replaces a hand-maintained ovmf_distro_aliases: map that lived in
// TWO byte-identical copies — charly's charly.yml and plugin-vm's build_defaults.yml —
// so every new distro had to be added twice, and adding it to only one was silent.
func TestDistroOvmfFamilies(t *testing.T) {
	// Every family assignment carried over from the old alias table.
	for id, want := range map[string]string{
		"fedora": "fedora", "rhel": "fedora", "centos": "fedora", "rocky": "fedora",
		"debian": "debian", "ubuntu": "debian",
		"arch": "arch", "manjaro": "arch", "endeavouros": "arch",
	} {
		if got := DistroOvmfFamilies[id]; got != want {
			t.Errorf("DistroOvmfFamilies[%q] = %q, want %q", id, got, want)
		}
	}

	// cachyos gains a family it never had: the alias table predated the distro, so a
	// CachyOS host fell through to the all-families union.
	if got := DistroOvmfFamilies["cachyos"]; got != "arch" {
		t.Errorf("DistroOvmfFamilies[cachyos] = %q, want arch", got)
	}

	// The old table keyed "alma", which is not a #DistroID and so could never match a
	// validated distro — a dead row. The id is almalinux.
	if _, dead := DistroOvmfFamilies["alma"]; dead {
		t.Error("DistroOvmfFamilies still carries the dead \"alma\" key; the distro id is almalinux")
	}
	if got := DistroOvmfFamilies["almalinux"]; got != "fedora" {
		t.Errorf("DistroOvmfFamilies[almalinux] = %q, want fedora", got)
	}

	// ovmf_family is OPTIONAL, and the ABSENCE is meaningful: it keeps a distro on the
	// all-families fallback rather than pinning it to the wrong firmware layout.
	// archarm's aarch64 paths differ; alpine had no entry before either.
	for _, id := range []string{"archarm", "alpine"} {
		if fam, ok := DistroOvmfFamilies[id]; ok {
			t.Errorf("DistroOvmfFamilies[%q] = %q, want absent — an unproven family is worse "+
				"than the fallback", id, fam)
		}
	}

	// Every entry must be a family the OVMF path table actually knows.
	for id, fam := range DistroOvmfFamilies {
		switch fam {
		case "fedora", "arch", "debian":
		default:
			t.Errorf("DistroOvmfFamilies[%q] = %q, which is not an OVMF family", id, fam)
		}
	}

	// And every keyed id must be a real distro id, or the row is dead like "alma" was.
	for id := range DistroOvmfFamilies {
		if _, ok := DistroFormats[id]; !ok {
			t.Errorf("DistroOvmfFamilies has %q, which is not a #DistroID — a row that can "+
				"never match", id)
		}
	}
}
