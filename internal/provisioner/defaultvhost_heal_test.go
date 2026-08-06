package provisioner

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The embedded copy is what a running panel installs; assets/nginx is what
// servika-install.sh copies onto a fresh host. They must not drift, or a new
// install and a healed one would end up serving different catch-all vhosts.
func TestEmbeddedDefaultVhostsMatchTheShippedAssets(t *testing.T) {
	for _, tc := range []struct {
		asset    string
		embedded string
	}{
		{"_default80.conf", default80Conf},
		{"_default443.conf", default443Conf},
	} {
		onDisk, err := os.ReadFile(filepath.Join("..", "..", "assets", "nginx", tc.asset))
		if err != nil {
			t.Fatalf("read assets/nginx/%s: %v", tc.asset, err)
		}
		if string(onDisk) != tc.embedded {
			t.Errorf("assets/nginx/%s and the embedded copy have diverged; copy the asset into internal/provisioner/nginx/ and append the old hash to the known list", tc.asset)
		}
	}
}

// The heal writes the current text. If that text's own hash were missing from
// the known list, the next boot would read back its own output, fail to
// recognise it, and warn about an operator edit that never happened.
func TestCurrentDefaultVhostsAreListedAsKnown(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		known   []string
	}{
		{"_default80.conf", default80Conf, knownDefault80},
		{"_default443.conf", default443Conf, knownDefault443},
	} {
		if !slices.Contains(tc.known, contentHash(tc.content)) {
			t.Errorf("%s: current content hash %s is not in the known list", tc.name, contentHash(tc.content))
		}
	}
}

func TestDecideVhostAction(t *testing.T) {
	const wanted = "server { listen 80; }\n"
	shipped := "server { listen 80; } # an older release\n"
	known := []string{contentHash(wanted), contentHash(shipped)}

	for _, tc := range []struct {
		name     string
		existing string
		exists   bool
		want     vhostAction
	}{
		{"absent file is installed", "", false, vhostInstall},
		{"current text is left alone", wanted, true, vhostUpToDate},
		{"previously shipped text is brought forward", shipped, true, vhostReplace},
		{"operator edit is preserved", wanted + "# our own location\n", true, vhostKeepEdited},
		// An empty but PRESENT file is not a shipped version, so it counts as an
		// edit. Treating it as absent would let the heal overwrite whatever an
		// operator deliberately blanked.
		{"blanked file counts as an edit", "", true, vhostKeepEdited},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := decideVhostAction(tc.existing, tc.exists, wanted, known); got != tc.want {
				t.Errorf("decideVhostAction = %v, want %v", got, tc.want)
			}
		})
	}
}
