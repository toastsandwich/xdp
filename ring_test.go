package xdp

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

var (
	mem      []byte
	ring     *Ring
	dummyOff unix.XDPRingOffset
	base     uintptr
)

func setupRing() {
	mem = make([]byte, 4096)
	base = uintptr(unsafe.Pointer(&mem[0]))

	dummyOff = unix.XDPRingOffset{
		Producer: 0x1F,
		Consumer: 0x2F,
		Desc:     0x3F,
	}

	UnsaferingsizeOverride(32)

	ring = NewRing(base, &dummyOff)
}

func TestSuiteRing(t *testing.T) {
	setupRing()
	t.Run("TestRingInit", testRingInit)
	t.Run("TestRingProduce", testRingProduce)
	t.Run("TestRingConsume", testRingConsume)
}

func testRingInit(t *testing.T) {
	expProducer := base + uintptr(dummyOff.Producer)
	expConsumer := base + uintptr(dummyOff.Consumer)
	expDesc := base + uintptr(dummyOff.Desc)

	if uintptr(unsafe.Pointer(ring.Producer)) != expProducer {
		t.Errorf("expected Producer at %#x, got %#x", expProducer, uintptr(unsafe.Pointer(ring.Producer)))
	}

	if uintptr(unsafe.Pointer(ring.Consumer)) != expConsumer {
		t.Errorf("expected Consumer at %#x, got %#x", expConsumer, uintptr(unsafe.Pointer(ring.Consumer)))
	}

	if uintptr(unsafe.Pointer(ring.Desc)) != expDesc {
		t.Errorf("expected Desc at %#x, got %#x", expDesc, uintptr(unsafe.Pointer(ring.Desc)))
	}

	if ring.mask != 31 {
		t.Errorf("expected mask=31, got %d", ring.mask)
	}
}

func testRingConsume(t *testing.T) {
	if !ring.Write(&unix.XDPDesc{
		Addr: 0x41,
		Len:  2,
	}) {
		t.Fatal("did not write to ring")
	}
	xdpDescPtr := ring.Read()
	prevPtr := ring.getDesc(int((*ring.Consumer - 1) & ring.mask))
	if p1, p2 := uintptr(unsafe.Pointer(xdpDescPtr)), uintptr(unsafe.Pointer(prevPtr)); p1 != p2 {
		t.Errorf("Consume logic failed address are different expected=%d, got=%d", p1, p2)
	}
}

func testRingProduce(t *testing.T) {
	expectedConsumePtr := uintptr(unsafe.Pointer(ring.getDesc(int(*ring.Producer & ring.mask))))
	if !ring.Write(&unix.XDPDesc{
		Addr: 0xFF,
		Len:  4,
	}) {
		t.Fatal("Didnot write to Ring")
	}

	gotConsumePtr := uintptr(unsafe.Pointer(ring.getDesc(int((*ring.Producer - 1) & ring.mask))))

	if expectedConsumePtr != gotConsumePtr {
		t.Errorf("Producee logic failed addresses are different expeceted=%d, got=%d", expectedConsumePtr, gotConsumePtr)
	}
}
