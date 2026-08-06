package php

import (
	"strings"
	"testing"

	"servika/internal/phpversion"
)

func discovered(version, resource string, loaded bool) phpversion.Version {
	return phpversion.Version{
		VersionMetadata: phpversion.VersionMetadata{Version: version, Resource: resource},
		Loaded:          loaded,
		Service:         "php-fpm",
	}
}

// The create form offers a version number and nothing else. Every label that
// used to ride along was wrong: the Remi one repeated its own name and called
// current releases legacy, the AppStream one promised OPcache that the base
// package set does not install.
func TestTheCreateFormIsOfferedNoSourceLabel(t *testing.T) {
	choices := runtimeChoices([]phpversion.Version{
		discovered("8.3", "appstream", true),
		discovered("8.2", "remi", true),
	})
	if len(choices) != 2 {
		t.Fatalf("got %d choices, want 2", len(choices))
	}
	for _, choice := range choices {
		if choice.Description != "" {
			t.Errorf("PHP %s carries the label %q", choice.Version, choice.Description)
		}
	}
}

// The per-domain PHP screen renders this label as the only indication of where a
// version came from, so stripping it there too would lose real information.
func TestThePerDomainScreenKeepsItsSourceLabel(t *testing.T) {
	choices := runtimeChoicesWithSource([]phpversion.Version{
		discovered("8.3", "appstream", true),
		discovered("8.2", "remi", true),
	})
	want := map[string]string{"8.3": "AppStream", "8.2": "Remi"}
	if len(choices) != len(want) {
		t.Fatalf("got %d choices, want %d", len(choices), len(want))
	}
	for _, choice := range choices {
		if choice.Description != want[choice.Version] {
			t.Errorf("PHP %s labelled %q, want %q", choice.Version, choice.Description, want[choice.Version])
		}
	}
}

// The label names the repository and stops. Anything longer is what went wrong
// before: a doubled name, an OPcache claim, or a legacy warning on a current
// release.
func TestTheSourceLabelSaysOnlyWhereARuntimeCameFrom(t *testing.T) {
	for _, resource := range []string{"appstream", "remi", "", "something-else"} {
		label := sourceLabel(resource)
		if label != "AppStream" && label != "Remi" {
			t.Errorf("sourceLabel(%q) = %q, want AppStream or Remi", resource, label)
		}
		for _, banned := range []string{"OPcache", "legacy", "Legacy", "·", "Remi · Remi"} {
			if strings.Contains(label, banned) {
				t.Errorf("sourceLabel(%q) = %q, which still carries %q", resource, label, banned)
			}
		}
	}
}

// A version installed from two repositories is discovered twice. Offering it
// twice would put two identical entries in the picker.
func TestARuntimeFoundTwiceIsOfferedOnce(t *testing.T) {
	unique := loadedRuntimes([]phpversion.Version{
		discovered("8.3", "appstream", true),
		discovered("8.3", "remi", true),
		discovered("8.1", "remi", false),
	})
	if len(unique) != 1 {
		t.Fatalf("got %d runtimes, want 1 (%v)", len(unique), unique)
	}
	if unique[0].Version != "8.3" || unique[0].Resource != "appstream" {
		t.Errorf("kept %s from %s, want 8.3 from appstream (the first discovered)",
			unique[0].Version, unique[0].Resource)
	}
}

// The static table is the fallback used when discovery finds nothing, so a label
// left behind there would resurface on exactly the installs that have no other
// source of truth.
func TestTheStaticFallbackCarriesNoLabel(t *testing.T) {
	for _, version := range InstalledVersions {
		if version.Description != "" {
			t.Errorf("PHP %s carries the label %q", version.Version, version.Description)
		}
	}
}
