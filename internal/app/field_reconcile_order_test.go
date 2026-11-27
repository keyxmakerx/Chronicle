package app

import (
	"os"
	"strings"
	"testing"
)

// The boot pass of the field reconcilers reads the system registry, so it
// has to start after the installed packages are loaded into it; started
// before, a restart converges nothing for packaged systems, which leaves a
// field a system newly marks gm_only visible on older types.
func TestFieldReconcilersStartAfterPackagesLoad(t *testing.T) {
	src, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	code := string(src)

	boot := strings.Index(code, "a.loadSystemsFromPackages(packages.NewPackageRepository(a.DB))")
	reconcile := strings.Index(code, "go reconcileFieldsFromSystems(")
	if boot < 0 || reconcile < 0 {
		t.Fatalf("boot package load at %d, boot reconcile at %d: both must be present", boot, reconcile)
	}
	if reconcile < boot {
		t.Error("the boot field reconcile starts before installed packages are loaded")
	}
	if n := strings.Count(code, "go reconcileFieldsFromSystems("); n != 1 {
		t.Errorf("boot field reconcile started %d times, want 1", n)
	}

	// The install hook loads first, then converges.
	hook := strings.Index(code, "packages.SetOnSystemInstall(")
	if hook < 0 {
		t.Fatal("system install hook not found")
	}
	rest := code[hook:]
	load := strings.Index(rest, "a.loadSystemsFromPackages(pkgService)")
	conv := strings.Index(rest, "reconcileFieldsFromSystems(context.Background(), entityService)")
	if load < 0 || conv < 0 || conv < load {
		t.Errorf("install hook: package load at %d, field reconcile at %d; the reconcile must follow the load", load, conv)
	}

	// No reconciler is called on its own outside the shared chain, where it
	// could run against a registry without the packaged systems.
	for _, fn := range []string{"reconcileFieldGMFlags(", "reconcileFieldOwnerOnlyFlags(", "reconcileFieldPlay(", "reconcileFieldChecks("} {
		if strings.Contains(code, fn) {
			t.Errorf("routes.go calls %s directly; use reconcileFieldsFromSystems after the package load", fn)
		}
	}
}
