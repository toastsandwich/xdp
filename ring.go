package xdp

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

type RingHeader struct {
	Producer uint32
	Consumer uint32
	Flags    uint32
	Padding  uint32
}

// this will Ring struct is used for Rx, Tx
type Ring struct {
	Header *RingHeader
	Buffer []unix.XDPDesc

	mask uint32
	raw  []byte
}

func NewRing(fd int, size uint64, offset int64, off unix.XDPRingOffset) (*Ring, error) {
	data, err := unix.Mmap(fd, offset, int(size), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_POPULATE)
	if err != nil {
		return nil, err
	}
	header := (*RingHeader)(unsafe.Pointer(&data[0]))
	bufferPtr := (*unix.XDPDesc)(unsafe.Add(unsafe.Pointer(header), unsafe.Sizeof(*header)))
	buffer := unsafe.Slice(bufferPtr, size*uint64(unsafe.Sizeof(unix.XDPDesc{})))
	header.Consumer = (uint32)(uintptr(unsafe.Pointer(bufferPtr)))
	header.Producer = header.Consumer
	return &Ring{
		Header: header,
		Buffer: buffer,
		raw:    data,
	}, nil
}

func (r *Ring) Close() error {
	return unix.Munmap(r.raw)
}

// Ring for Userspace memory
type URing struct {
	Header *RingHeader
	Buffer []int64

	raw  []byte
	mask uint32
}

// Fill Ring used by kernel
// Fill rings fills this with addresses to be used by Rx ring
func (r *URing) Consume() (int64, bool) {
	if r.Header.Consumer == r.Header.Producer {
		return -1, false // Ring is empty
	}

	addr := r.Buffer[r.Header.Consumer]
	r.Header.Consumer = (r.Header.Consumer + 1) & r.mask
	return addr, true
}

// Produce adds new address to ring buffer
func (r *URing) Produce(v int64) bool {
	if (r.Header.Producer+1)&r.mask == r.Header.Consumer {
		return false // ring is full cannot produce
	}

	r.Buffer[r.Header.Producer] = v
	r.Header.Producer = (r.Header.Producer + 1) & r.mask
	return true
}

func (r *URing) Close() error {
	return unix.Munmap(r.raw)
}

func NewURing(fd int, length uint64, offset int64, off unix.XDPRingOffset) (*URing, error) {
	data, err := unix.Mmap(fd, offset, int(length), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_POPULATE)
	if err != nil {
		return nil, err
	}
	header := (*RingHeader)(unsafe.Pointer(&data[0]))
	headerLen := unsafe.Sizeof(*header)
	bufferPtr := (*int64)(unsafe.Add(unsafe.Pointer(header), headerLen))
	bufferLen := (len(data) - int(headerLen)) / int(unsafe.Sizeof(int64(0)))
	buffer := unsafe.Slice(bufferPtr, bufferLen)
	header.Consumer = uint32(0)
	header.Producer = header.Consumer
	return &URing{
		Header: header,
		Buffer: buffer,
		raw:    data,
		mask:   uint32(length - 1),
	}, nil
}
