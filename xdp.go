package xdp

import (
	"fmt"
	"net"
	"unsafe"

	"golang.org/x/sys/unix"
)

var (
	chunksize  uint = 4096
	framecount uint = 4096
	ringsize   uint = 512
)

// Both should be called before creating your socket
func UnsafeChucksizeOverride(size uint) {
	chunksize = size
}

func UnsafeFramecountOverride(count uint) {
	framecount = count
}

func UnsaferingsizeOverride(size uint) {
	ringsize = size
}

type Socket interface {
	reactor() // reactor loop for epolls
} // for future when i include UDP support as well

/*
TODO
- sockets to have a umem file in /tmp
- creating a code geenerator for c code
*/

// XSK
// 1. Bind to a addr
// 2. Create UMEM
// 3. Setup RX TX FILL COMP rings
type SocketTCP struct {
	fd   int    // file decscriptor
	mem  []byte // memory space and all the rings will have addrs for this space
	offs unix.XDPMmapOffsets

	Iface uint32
	Proto uint32

	Rx *Ring
	Tx *Ring
	Fr *Ring
	Cr *Ring
}

// NewSocket only supports IPv4
func NewSocket(ifaceName string, proto int, queueID int) (Socket, error) {
	xsk := &SocketTCP{} // xskTCP
	var err error

	xsk.fd, err = unix.Socket(unix.AF_XDP, unix.SOCK_RAW, proto)
	if err != nil {
		return nil, err
	}

	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, err
	}
	xsk.Iface = uint32(iface.Index)

	xsk.mem = make([]byte, chunksize*framecount)

	umemReg := unix.XDPUmemReg{
		Addr: uint64(uintptr(unsafe.Pointer(&xsk.mem[0]))),
		Size: uint32(chunksize),
		Len:  uint64(chunksize * framecount),
	}

	_, _, errno := unix.Syscall6(
		unix.SYS_SETSOCKOPT,
		uintptr(xsk.fd),
		uintptr(unix.SOL_XDP),
		uintptr(unix.XDP_UMEM_REG),
		uintptr(unsafe.Pointer(&umemReg)),
		uintptr(unsafe.Sizeof(umemReg)),
		0,
	)
	if errno != 0 {
		return nil, errnoError("set SYS_SOCKOPT", errno)
	}

	// inform kernel about ring size
	if err := unix.SetsockoptInt(xsk.fd, unix.SOL_XDP, unix.XDP_RX_RING, int(ringsize)); err != nil {
		return nil, err
	}

	if err := unix.SetsockoptInt(xsk.fd, unix.SOL_XDP, unix.XDP_TX_RING, int(ringsize)); err != nil {
		return nil, err
	}

	if err := unix.SetsockoptInt(xsk.fd, unix.SOL_XDP, unix.XDP_UMEM_FILL_RING, int(ringsize)); err != nil {
		return nil, err
	}

	if err := unix.SetsockoptInt(xsk.fd, unix.SOL_XDP, unix.XDP_UMEM_COMPLETION_RING, int(ringsize)); err != nil {
		return nil, err
	}

	xsk.offs = unix.XDPMmapOffsets{}
	offsetsSize := unsafe.Sizeof(xsk.offs)
	_, _, errno = unix.Syscall6(
		unix.SYS_GETSOCKOPT,
		uintptr(xsk.fd),
		uintptr(unix.SOL_XDP),
		uintptr(unix.XDP_MMAP_OFFSETS),
		uintptr(unsafe.Pointer(&xsk.offs)),
		uintptr(unsafe.Pointer(&offsetsSize)),
		0,
	)
	if errno != 0 {
		return nil, errnoError("get SYS_SOCKOPT", errno)
	}

	rxMmap, err := unix.Mmap(xsk.fd, unix.XDP_PGOFF_RX_RING, int(xsk.offs.Rx.Desc)+int(ringsize)*int(unsafe.Sizeof(xsk.offs.Rx.Desc)), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_POPULATE)
	if err != nil {
		return nil, detailError("creating mmap for rx", err)
	}
	rxBase := uintptr(unsafe.Pointer(&rxMmap[0]))

	// create rings
	xsk.Rx = NewRing(rxBase, &xsk.offs.Rx)

	txMmap, err := unix.Mmap(xsk.fd, unix.XDP_PGOFF_TX_RING, int(xsk.offs.Tx.Desc)+int(ringsize)*int(unsafe.Sizeof(xsk.offs.Tx.Desc)), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_POPULATE)
	if err != nil {
		return nil, detailError("creating mmap for tx", err)
	}
	txBase := uintptr(unsafe.Pointer(&txMmap[0]))
	xsk.Tx = NewRing(txBase, &xsk.offs.Tx)

	fillMmap, err := unix.Mmap(xsk.fd, unix.XDP_UMEM_PGOFF_FILL_RING, int(xsk.offs.Tx.Desc)+int(ringsize)*int(unsafe.Sizeof(xsk.offs.Fr.Desc)), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_POPULATE)
	if err != nil {
		return nil, detailError("creating mmap for fr", err)
	}
	fillUmemBase := uintptr(unsafe.Pointer(&fillMmap[0]))
	xsk.Fr = NewRing(fillUmemBase, &xsk.offs.Fr)

	compUmemMmap, err := unix.Mmap(xsk.fd, unix.XDP_UMEM_COMPLETION_RING, int(xsk.offs.Tx.Desc)+int(ringsize)*int(unsafe.Sizeof(xsk.offs.Cr.Desc)), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_POPULATE)
	if err != nil {
		return nil, detailError("creating mmap for cr", err)
	}
	compBase := uintptr(unsafe.Pointer(&compUmemMmap[0]))
	xsk.Cr = NewRing(compBase, &xsk.offs.Cr)

	return xsk, nil
}

func (x *SocketTCP) reactor() {
	xdpDescPtr := x.Rx.Read()
	addr := xdpDescPtr.Addr // still incomplete will throw error
	x.Fr.Write(xdpDescPtr)
}

func detailError(msg string, err error) error {
	return fmt.Errorf("%s: %v", msg, err)
}

func errnoError(msg string, errno unix.Errno) error {
	return fmt.Errorf("%s: %v(=%d)", msg, errno, errno)
}
