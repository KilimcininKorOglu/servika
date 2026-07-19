package resourcelimit

import (
	"context"
	"os"
	"reflect"
	"testing"
)

// TestQuotaLimitArgs verifies that the xfs_quota arg slice is constructed correctly:
// soft=hard*0.95, 0=unlimited, -u user quota, root mount. No shell — arg slice
// integrity is critical.
func TestQuotaLimitArgs(t *testing.T) {
	cases := []struct {
		sk           string
		diskMB       int
		inode        int
		wantLimitArg string
	}{
		// Full limits: soft = 95% of hard.
		{"c_example", 5120, 500000, "limit -u bsoft=4864m bhard=5120m isoft=475000 ihard=500000 c_example"},
		// Disk limit + inode unlimited (0).
		{"c_foo", 1024, 0, "limit -u bsoft=972m bhard=1024m isoft=0 ihard=0 c_foo"},
		// Both unlimited (0 = no limit).
		{"c_bar", 0, 0, "limit -u bsoft=0m bhard=0m isoft=0 ihard=0 c_bar"},
		// Inode-only limit.
		{"c_baz", 0, 100000, "limit -u bsoft=0m bhard=0m isoft=95000 ihard=100000 c_baz"},
		// Negative values clamp to 0.
		{"c_neg", -5, -9, "limit -u bsoft=0m bhard=0m isoft=0 ihard=0 c_neg"},
	}
	for _, c := range cases {
		got := quotaLimitArgs(c.sk, c.diskMB, c.inode)
		want := []string{"-x", "-c", c.wantLimitArg, "/"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("quotaLimitArgs(%q,%d,%d)\n  got  = %#v\n  want = %#v", c.sk, c.diskMB, c.inode, got, want)
		}
	}
}

// TestReQuotaSK verifies the system user allowlist accepts valid c_<slug> and rejects
// injection / invalid names.
func TestReQuotaSK(t *testing.T) {
	valid := []string{"c_foo", "c_regular_permanent_test_local", "c_a1b2c3", "c_x"}
	for _, v := range valid {
		if !reQuotaSK.MatchString(v) {
			t.Errorf("reQuotaSK rejected valid system user: %q", v)
		}
	}
	invalid := []string{
		"", "root", "foo", "c_", "c_Foo", "c_foo bar", "c_foo;rm -rf /",
		"c_foo`id`", "c_foo\nx", "../c_foo", "c_foo/../bar", "admin",
	}
	for _, v := range invalid {
		if reQuotaSK.MatchString(v) {
			t.Errorf("reQuotaSK accepted invalid/dangerous system user: %q", v)
		}
	}
}

// TestEffectiveQuota verifies the override > plan > default resolution logic.
func TestEffectiveQuota(t *testing.T) {
	cases := []struct {
		name                string
		dOver, iOver        int
		planAssigned        bool
		pDisk, pInode       int
		wantDisk, wantInode int
	}{
		{"no plan → default", 0, 0, false, 0, 0, defaultDiskMB, defaultInode},
		{"plan assigned, explicitly unlimited", 0, 0, true, 0, 0, 0, 0},
		{"plan values", 0, 0, true, 1024, 50000, 1024, 50000},
		{"disk override beats plan", 2048, 0, true, 1024, 50000, 2048, 50000},
		{"inode override beats planless", 0, 99999, false, 0, 0, defaultDiskMB, 99999},
		{"both overrides", 3072, 123456, true, 1024, 50000, 3072, 123456},
	}
	for _, c := range cases {
		gd, gi := effectiveQuota(c.dOver, c.iOver, c.planAssigned, c.pDisk, c.pInode)
		if gd != c.wantDisk || gi != c.wantInode {
			t.Errorf("%s: effectiveQuota=(%d,%d) want=(%d,%d)", c.name, gd, gi, c.wantDisk, c.wantInode)
		}
	}
}

// TestApplyQuotaNoquotaGracefulLive verifies that ApplyQuota does NOT return an error on
// a noquota filesystem (log + return nil = graceful skip). Only runs with KOTA_LIVE=1
// because it calls the real xfs_quota binary.
func TestApplyQuotaNoquotaGracefulLive(t *testing.T) {
	if os.Getenv("KOTA_LIVE") != "1" {
		t.Skip("run with KOTA_LIVE=1 on real (noquota) filesystem")
	}
	acc, enf := mountQuotaActive()
	t.Logf("mountQuotaActive(): accounting=%v enforcement=%v", acc, enf)
	if err := ApplyQuota(context.Background(), "c_kotatest_noquota", 1024, 50000); err != nil {
		t.Fatalf("ApplyQuota returned an ERROR on noquota filesystem (expected graceful skip): %v", err)
	}
	t.Log("ApplyQuota on noquota: graceful skip, err=nil ✓")
}
