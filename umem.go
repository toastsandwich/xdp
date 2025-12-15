package xdp

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

var pg = os.Getpagesize()

// pageAlign rounds it to multiple of page size
func pageAlign(len int) int {
	return ((len + pg - 1) / pg) * pg
}

// A contagious memory for socket to use. Can be shared by multiple sockets
type UMem struct {
	arena []byte
	ptr   unsafe.Pointer // will be used for pointer arithematics
}

func NewUMem(len int) (*UMem, error) {
	var err error
	umem := &UMem{}

	len = pageAlign(len)
	umem.arena, err = unix.Mmap(-1, 0, len, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_ANON|unix.MAP_PRIVATE|unix.MAP_POPULATE)
	if err != nil {
		return nil, err
	}

	umem.ptr = unsafe.Pointer(&umem.arena[0])
	return umem, nil
}

func (u *UMem) Close() error {
	return unix.Munmap(u.arena)
}
