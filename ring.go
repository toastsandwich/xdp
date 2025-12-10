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

	raw []byte
}

func (r *Ring) Close() error {
	return unix.Munmap(r.raw)
}

// Ring for Userspace memory
type URing struct {
	Header *RingHeader
	Buffer []int64

	raw []byte
}

func (r *URing) Close() error {
	return unix.Munmap(r.raw)
}

func NewRing(fd int, offset int64, len int, off unix.XDPRingOffset) (*Ring, error) {
	data, err := unix.Mmap(fd, offset, len, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_POPULATE)
	if err != nil {
		return nil, err
	}
	header := (*RingHeader)(unsafe.Pointer(&data[0]))
	bufferPtr := (*unix.XDPDesc)(unsafe.Add(unsafe.Pointer(header), unsafe.Sizeof(*header)))
	buffer := unsafe.Slice(bufferPtr, off.Desc+uint64(RINGSIZE)*uint64(unsafe.Sizeof(unix.XDPDesc{})))

	return &Ring{
		Header: header,
		Buffer: buffer,
		raw:    data,
	}, nil
}

func NewURing(fd int, offset int64, len int, off unix.XDPRingOffset) (*URing, error) {
	data, err := unix.Mmap(fd, offset, len, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_POPULATE)
	if err != nil {
		return nil, err
	}
	header := (*RingHeader)(unsafe.Pointer(&data[0]))
	bufferPtr := (*int64)(unsafe.Add(unsafe.Pointer(header), unsafe.Sizeof(*header)))
	buffer := unsafe.Slice(bufferPtr, off.Desc+uint64(RINGSIZE)*uint64(unsafe.Sizeof(int64(0))))
	return &URing{
		Header: header,
		Buffer: buffer,
		raw:    data,
	}, nil
}
