package usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The real report from importing strongswan out of sid, which is what the
// resolver has to read to work out what else has to come along.
const strongswanUnmet = `The following packages have unmet dependencies:
 charon-systemd : Depends: libstrongswan (= 6.1.0-2) but it is not going to be installed
                  Depends: strongswan-swanctl but it is not going to be installed
                  Depends: libc6 (>= 2.34) but it is not going to be installed
                  Depends: libsystemd0 but it is not going to be installed
                  Depends: strongswan-libcharon (>= 6.1.0) but it is not going to be installed
                  Conflicts: strongswan-charon but 6.1.0-2 is to be installed
 strongswan : Depends: strongswan-swanctl but it is not going to be installed or
                       strongswan-starter but it is not going to be installed
 strongswan-charon : PreDepends: debconf but it is not going to be installed or
                                 debconf-2.0
                     Depends: iproute2 but it is not going to be installed or
                              iproute but it is not installable
                     Depends: libstrongswan (= 6.1.0-2) but it is not going to be installed`

func TestParseUnmetDependencies(t *testing.T) {
	deps := parseUnmetDependencies(strongswanUnmet)

	names := map[string]string{}
	for _, dep := range deps {
		names[dep.Package] = dep.Constraint
	}

	assert.Equal(t, "= 6.1.0-2", names["libstrongswan"])
	assert.Equal(t, ">= 2.34", names["libc6"])
	assert.Equal(t, ">= 6.1.0", names["strongswan-libcharon"])
	assert.Contains(t, names, "strongswan-swanctl")
	assert.Contains(t, names, "libsystemd0")
	// PreDepends counts too: it is just as unsatisfied.
	assert.Contains(t, names, "debconf")

	// A conflict is not a missing dependency. Two packages that conflict are
	// not short of anything, they simply cannot be installed together, which
	// a repository never asks of them.
	assert.NotContains(t, names, "strongswan-charon")
}

// Each dependency is attributed to the package that needs it, so the report
// can say who is dragging what in.
func TestParseUnmetDependencies_AttributesToTheWantingPackage(t *testing.T) {
	deps := parseUnmetDependencies(strongswanUnmet)

	wantedBy := map[string]string{}
	for _, dep := range deps {
		if _, seen := wantedBy[dep.Package]; !seen {
			wantedBy[dep.Package] = dep.Wanted
		}
	}

	assert.Equal(t, "charon-systemd", wantedBy["libsystemd0"])
	assert.Equal(t, "strongswan-charon", wantedBy["debconf"])
}

func TestParseUnmetDependencies_NoReport(t *testing.T) {
	assert.Empty(t, parseUnmetDependencies(""))
	assert.Empty(t, parseUnmetDependencies("Reading package lists...\nBuilding dependency tree..."))
}

func TestFormatBytes(t *testing.T) {
	assert.Equal(t, "?", formatBytes(0))
	assert.Equal(t, "512 B", formatBytes(512))
	assert.Equal(t, "412 kB", formatBytes(412_000))
	assert.Equal(t, "4.8 MB", formatBytes(4_800_000))
	assert.Equal(t, "1.2 GB", formatBytes(1_200_000_000))
}

func TestImportSet_RepresentativesNamesOnePackagePerAddedSource(t *testing.T) {
	set := newImportSet()
	set.add(&sourcePackage{Name: "strongswan", Binaries: []string{"strongswan", "libstrongswan"}, Requested: true})
	set.add(&sourcePackage{Name: "libnl3", Binaries: []string{"libnl-3-200", "libnl-3-dev"}})

	// What the maintainer asked for, plus one binary of each added source:
	// the repo worker expands it back to every binary of both sources.
	assert.Equal(t, []string{"strongswan", "libnl-3-200"}, set.representatives([]string{"strongswan"}))
}
