package xdp

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

type Ring struct {
	Consumer *uint32
	Producer *uint32

	Desc *unix.XDPDesc // pointer to first descriptor

	mask uint32
}

func NewRing(base uintptr, off *unix.XDPRingOffset) *Ring {
	return &Ring{
		Consumer: (*uint32)(unsafe.Pointer(base + uintptr(off.Consumer))),
		Producer: (*uint32)(unsafe.Pointer(base + uintptr(off.Producer))),
		Desc:     (*unix.XDPDesc)(unsafe.Pointer(base + uintptr(off.Desc))),
		mask:     uint32(ringsize - 1),
	}
}

func (r *Ring) getDesc(i int) *unix.XDPDesc {
	return (*unix.XDPDesc)(unsafe.Pointer(uintptr(unsafe.Pointer(r.Desc)) + uintptr(i)*unsafe.Sizeof(*r.Desc)))
}

func (r *Ring) Write(desc *unix.XDPDesc) bool {
	next := (*r.Producer + 1) & r.mask
	if next == *r.Consumer {
		return false // ring is full
	}
	targetPtr := r.getDesc(int(*r.Producer & r.mask))
	*targetPtr = *desc
	*r.Producer = next
	return true
}

func (r *Ring) Read() *unix.XDPDesc {
	if *r.Consumer == *r.Producer {
		return nil // ring is empty
	}
	desc := r.getDesc(int(*r.Consumer & r.mask))
	*r.Consumer++
	return desc
}
