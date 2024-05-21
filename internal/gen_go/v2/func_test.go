package gen_go_v2

import "testing"

func TestXxx(t *testing.T) {
	have := goEscape("default")
	if have != "default_" {
		t.Errorf(`"%s" != "default_"`, have)
	}
}
