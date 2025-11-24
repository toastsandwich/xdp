package xdp

import "testing"

func TestXDPInit(t *testing.T) {
	// implement iface to select the active eth iface
	_, err := NewSocket("enp3s0", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
}
