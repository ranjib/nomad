package driver

import (
	"testing"
)

func TestLXCDriver_Handle(t *testing.T) {
	t.Parallel()
	h := &lxcHandle{
		Name: "foo",
	}
	actual := h.ID()
	expected := "LXC:foo"
	if actual != expected {
		t.Errorf("Expected: `%s`, Found: `%s`", actual, expected)
	}
}
