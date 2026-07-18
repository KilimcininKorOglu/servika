package php

import (
	"strings"
	"testing"
)

func TestRenderPoolAlwaysConfinesTenantFilesystem(t *testing.T) {
	settings := Defaults()
	settings.OpenBasedir = ""

	pool, err := RenderPool("c_example_com", "/run/php-fpm", settings)
	if err != nil {
		t.Fatalf("RenderPool() error: %v", err)
	}
	if !strings.Contains(pool, "php_admin_value[open_basedir] = /home/c_example_com/:/tmp/") {
		t.Fatal("RenderPool() omitted the safe default open_basedir")
	}
}

func TestRenderPoolAllowsOnlyNonPrivilegedCustomPHPDirectives(t *testing.T) {
	settings := Defaults()
	settings.ExtraDirectives = "; custom limits\nphp_value[max_input_vars] = 2000\nphp_flag[expose_php] = Off"
	if _, err := RenderPool("c_example_com", "/run/php-fpm", settings); err != nil {
		t.Fatalf("RenderPool() rejected safe custom directives: %v", err)
	}

	unsafe := []string{
		"php_admin_value[memory_limit] = -1",
		"php_value[open_basedir] = /",
		"user = root",
		"[injected]",
	}
	for _, directive := range unsafe {
		t.Run(directive, func(t *testing.T) {
			settings.ExtraDirectives = directive
			if _, err := RenderPool("c_example_com", "/run/php-fpm", settings); err == nil {
				t.Fatalf("RenderPool() accepted unsafe directive %q", directive)
			}
		})
	}
}

func TestRenderPoolRejectsScalarLineInjection(t *testing.T) {
	settings := Defaults()
	settings.MemoryLimit = "256M\nuser = root"
	if _, err := RenderPool("c_example_com", "/run/php-fpm", settings); err == nil {
		t.Fatal("RenderPool() accepted a line break in a scalar setting")
	}
}
