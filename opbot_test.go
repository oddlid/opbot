package opbot

import (
	"testing"
)

func TestMatchMask(t *testing.T) {
	mask1 := "oddee!~Oddlid@192.168.3.17"
	rxs := []string{
		"*!*@*",
		"*!*Oddlid@*",
		"oddee!*@192.168.3.*",
		"odd*!*ddlid@*.3.17",
	}

	for _, rx := range rxs {
		if !matchMask(rx, mask1) {
			t.Errorf("%q did not match %q", rx, mask1)
		}
	}

	if matchMask("oddee!*elide@*", mask1) {
		t.Errorf("Should not match")
	}
}
