package xdp

import "testing"

func TestXDP(t *testing.T) {
	_, err := NewSocket("enp3s0")
	if err != nil {
		t.Fatal(err)
	}

}
