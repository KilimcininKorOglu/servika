package system

import (
	"slices"
	"testing"
)

// Real `sshd -T` output is lowercase and carries other keys that contain the word
// port; only the `port` directive itself may be read.
func TestPortDirectiveReadsOnlyThePortLine(t *testing.T) {
	const output = `port 2222
addressfamily any
listenaddress 0.0.0.0:2222
permitrootlogin no
x11displayoffset 10
gatewayports no
`
	var found []string
	for _, match := range portDirectivePattern.FindAllStringSubmatch(output, -1) {
		found = append(found, match[1])
	}
	if !slices.Equal(found, []string{"2222"}) {
		t.Errorf("read %v, want only the port directive", found)
	}
}

// A half-finished migration keeps both ports, and the warning has to stay up
// while 22 is one of them.
func TestPortDirectiveReadsEveryPortLine(t *testing.T) {
	var ports []string
	for _, match := range portDirectivePattern.FindAllStringSubmatch("port 22\nport 2222\n", -1) {
		ports = append(ports, match[1])
	}
	if !slices.Equal(ports, []string{"22", "2222"}) {
		t.Errorf("read %v, want both ports", ports)
	}
}

// A `port` key with no value must not swallow the number on the following line.
func TestPortDirectiveDoesNotSpanLines(t *testing.T) {
	if portDirectivePattern.MatchString("port\n2222\n") {
		t.Error("the pattern joined a bare port key to the next line")
	}
}

func TestUniqueSortedRemovesRepeats(t *testing.T) {
	if got := uniqueSorted([]int{2222, 22, 2222, 22}); !slices.Equal(got, []int{22, 2222}) {
		t.Errorf("uniqueSorted = %v, want [22 2222]", got)
	}
	if got := uniqueSorted(nil); got != nil {
		t.Errorf("uniqueSorted(nil) = %v, want nil", got)
	}
}

// uniqueSorted must not reorder its caller's slice, which is the detection
// result being handed back.
func TestUniqueSortedLeavesTheInputAlone(t *testing.T) {
	input := []int{2222, 22}
	uniqueSorted(input)
	if !slices.Equal(input, []int{2222, 22}) {
		t.Errorf("the input was modified: %v", input)
	}
}

// Both address families appear in `ss -lntp`, and the IPv6 row carries colons of
// its own before the port.
func TestListenPortIsReadFromBothAddressFamilies(t *testing.T) {
	for name, row := range map[string]string{
		"ipv4": `LISTEN 0 128 0.0.0.0:2222 0.0.0.0:* users:(("sshd",pid=1,fd=3))`,
		"ipv6": `LISTEN 0 128 [::]:2222 [::]:* users:(("sshd",pid=1,fd=4))`,
	} {
		t.Run(name, func(t *testing.T) {
			match := listenPortPattern.FindStringSubmatch(row)
			if match == nil || match[1] != "2222" {
				t.Errorf("read %v from %q, want 2222", match, row)
			}
		})
	}
}
