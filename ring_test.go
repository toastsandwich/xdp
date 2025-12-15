package xdp

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRing(t *testing.T) {
	f, err := os.CreateTemp("", ".ringmmap")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	f.Truncate(512) // set ring size

	ring, err := NewURing(int(f.Fd()), 512, 0, unix.XDPRingOffset{})
	if err != nil {
		t.Fatal(err)
	}
	defer ring.Close()

	ring.Produce(10001)
	ring.Produce(101)
	ring.Produce(120)
	ring.Consume()
	ring.Consume()

	fmt.Println(ring.Buffer)
	fmt.Println(ring.raw)
	fmt.Println(ring.Header)
}
