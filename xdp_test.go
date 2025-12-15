package xdp

import "testing"

func TestXDP(t *testing.T) {
	xsk, err := NewSocket(nil, "enp3s0")
	if err != nil {
		t.Fatal(err)
	}
	defer xsk.Close()
}
