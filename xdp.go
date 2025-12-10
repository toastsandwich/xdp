package xdp

import (
	"fmt"
	"net"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

var (
	CHUNCKSIZE  uint32 = 4096
	CHUNCKCOUNT uint32 = 4096
	RINGSIZE    uint32 = 512
)

// As of now this socket will just recieve packets
type Socket struct {
	fd   int
	umem *UMem

	rx, tx     *Ring
	fill, comp *URing
}

func NewSocket(iface string) (*Socket, error) {
	var (
		xsk = &Socket{}
		err error
	)
	xsk.fd, err = syscall.Socket(unix.AF_XDP, unix.SOCK_RAW, 0)
	if err != nil {
		return nil, ErrMsg("syscall.Socket() failed to create socket with error", err)
	}

	xsk.umem, err = NewUMem(int(CHUNCKCOUNT * CHUNCKSIZE))
	if err != nil {
		return nil, err
	}

	// register the user mem to xsk
	umemReg := unix.XDPUmemReg{
		Addr: uint64(uintptr(xsk.umem.ptr)),
		Size: CHUNCKSIZE,
		Len:  uint64(CHUNCKSIZE * CHUNCKCOUNT),
	}

	umemRegSize := unsafe.Sizeof(umemReg)
	rc, _, errno := syscall.Syscall6(
		unix.SYS_SETSOCKOPT,
		uintptr(xsk.fd),
		uintptr(unix.SOL_XDP),
		uintptr(unix.XDP_UMEM_REG),
		uintptr(unsafe.Pointer(&umemReg)),
		umemRegSize,
		0,
	)
	if rc != 0 {
		return nil, ErrMsg("syscall.Sycall6() failed to set umem to xdp socket", errno)
	}

	// inform kernel about ring size
	rc, _, errno = syscall.Syscall6(
		unix.SYS_SETSOCKOPT,
		uintptr(xsk.fd),
		uintptr(unix.SOL_XDP),
		uintptr(unix.XDP_RX_RING),
		uintptr(unsafe.Pointer(&RINGSIZE)),
		uintptr(unsafe.Sizeof(RINGSIZE)),
		0,
	)
	if rc != 0 {
		return nil, ErrMsg("syscall.Sycall() failed to informal kernel about Rx ring size", errno)
	}

	rc, _, errno = syscall.Syscall6(
		unix.SYS_SETSOCKOPT,
		uintptr(xsk.fd),
		uintptr(unix.SOL_XDP),
		uintptr(unix.XDP_TX_RING),
		uintptr(unsafe.Pointer(&RINGSIZE)),
		uintptr(unsafe.Sizeof(RINGSIZE)),
		0,
	)
	if rc != 0 {
		return nil, ErrMsg("syscall.Sycall() failed to informal kernel about Tx ring size", errno)
	}

	rc, _, errno = syscall.Syscall6(
		unix.SYS_SETSOCKOPT,
		uintptr(xsk.fd),
		uintptr(unix.SOL_XDP),
		uintptr(unix.XDP_UMEM_FILL_RING),
		uintptr(unsafe.Pointer(&RINGSIZE)),
		uintptr(unsafe.Sizeof(RINGSIZE)),
		0,
	)
	if rc != 0 {
		return nil, ErrMsg("syscall.Sycall() failed to informal kernel about Umem Fill ring size", errno)
	}

	rc, _, errno = syscall.Syscall6(
		unix.SYS_SETSOCKOPT,
		uintptr(xsk.fd),
		uintptr(unix.SOL_XDP),
		uintptr(unix.XDP_UMEM_COMPLETION_RING),
		uintptr(unsafe.Pointer(&RINGSIZE)),
		uintptr(unsafe.Sizeof(RINGSIZE)),
		0,
	)
	if rc != 0 {
		return nil, ErrMsg("syscall.Sycall6() failed to informal kernel about Umem Comp ring size", errno)
	}

	// now set offsets for all the rings
	offs := unix.XDPMmapOffsets{}
	offsSize := unsafe.Sizeof(offs)
	rc, _, errno = syscall.Syscall6(
		unix.SYS_GETSOCKOPT,
		uintptr(xsk.fd),
		uintptr(unix.SOL_XDP),
		uintptr(unix.XDP_MMAP_OFFSETS),
		uintptr(unsafe.Pointer(&offs)),
		uintptr(unsafe.Pointer(&offsSize)), 0,
	)
	if rc != 0 {
		return nil, ErrMsg("syscall.Syscall6() failed to get offsets for rings in umem", errno)
	}

	xsk.rx, err = NewRing(xsk.fd, unix.XDP_PGOFF_RX_RING, int(RINGSIZE), offs.Rx)
	if err != nil {
		return nil, ErrMsg("NewRing() failed to creating rx ring", err)
	}
	defer xsk.rx.Close()

	xsk.fill, err = NewURing(xsk.fd, unix.XDP_UMEM_PGOFF_FILL_RING, int(RINGSIZE), offs.Fr)
	if err != nil {
		return nil, ErrMsg("NewURing() failed to creating fr ring", err)
	}
	defer xsk.fill.Close()

	fmt.Println(xsk.rx.Header)

	xsk.comp, err = NewURing(xsk.fd, unix.XDP_UMEM_PGOFF_COMPLETION_RING, int(RINGSIZE), offs.Cr)
	if err != nil {
		return nil, ErrMsg("NewURing() failed to creating cr ring", err)
	}
	defer xsk.comp.Close()

	ifaceIdx, err := ifaceIdx(iface)
	if err != nil {
		return nil, ErrMsg("Failed to get iface index", err)
	}

	addr := unix.SockaddrXDP{
		Ifindex: uint32(ifaceIdx),
	}

	if err := unix.Bind(xsk.fd, &addr); err != nil {
		return nil, ErrMsg("unix.Bind() failed to bind xdp addr and xdp socket", err)
	}

	return xsk, nil
}

func ErrMsg(msg string, err error) error {
	switch err.(type) {
	case unix.Errno:
		return fmt.Errorf("%s: %v(%d)", msg, err, err)
	default:
		return fmt.Errorf("%s: %v", msg, err)
	}
}

func ifaceIdx(ifaceName string) (int, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return -1, err
	}
	return iface.Index, nil
}
