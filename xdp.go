package xdp

import (
	"fmt"
	"net"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Options for socket
const (
	O_RD   = (iota + 1) << 0 // Read Only
	O_WR                     // Write only
	O_RDWR                   // Read-Write

)

var DefaultOption = Options{
	ChunkSize:  4096,
	ChunkCount: 4096,
	RingSize:   512,
}

// Options struct contains field to define count, size of chunk/frame for umem.
// Ringsize for Rx, Tx, Fill, Comp rings.
// Take a note that all field values must be always a power of 2
type Options struct {
	ChunkSize  uint32
	ChunkCount uint32
	RingSize   uint32
}

// As of now this socket will just recieve packets
type Socket struct {
	Opts *Options
	fd   int
	umem *UMem

	rx, tx     *Ring
	fill, comp *URing
}

func NewSocket(opts *Options, iface string) (*Socket, error) {
	if opts == nil {
		opts = &DefaultOption
	}
	var (
		xsk = &Socket{
			Opts: opts,
		}
		err error
	)
	xsk.fd, err = syscall.Socket(unix.AF_XDP, unix.SOCK_RAW, 0)
	if err != nil {
		return nil, ErrMsg("syscall.Socket() failed to create socket with error", err)
	}

	xsk.umem, err = NewUMem(int(xsk.Opts.ChunkCount * xsk.Opts.ChunkSize))
	if err != nil {
		return nil, ErrMsg("Failed to create memory region", err)
	}

	// register the user mem to xsk
	umemReg := unix.XDPUmemReg{
		Addr: uint64(uintptr(xsk.umem.ptr)),
		Size: xsk.Opts.ChunkSize,
		Len:  uint64(xsk.Opts.ChunkSize * xsk.Opts.ChunkCount),
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
		uintptr(unsafe.Pointer(&xsk.Opts.RingSize)),
		uintptr(unsafe.Sizeof(xsk.Opts.RingSize)),
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
		uintptr(unsafe.Pointer(&xsk.Opts.RingSize)),
		uintptr(unsafe.Sizeof(xsk.Opts.RingSize)),
		0,
	)
	if rc != 0 {
		return nil, ErrMsg("syscall.Sycall6() failed to informal kernel about Tx ring size", errno)
	}

	rc, _, errno = syscall.Syscall6(
		unix.SYS_SETSOCKOPT,
		uintptr(xsk.fd),
		uintptr(unix.SOL_XDP),
		uintptr(unix.XDP_UMEM_FILL_RING),
		uintptr(unsafe.Pointer(&xsk.Opts.RingSize)),
		uintptr(unsafe.Sizeof(xsk.Opts.RingSize)),
		0,
	)
	if rc != 0 {
		return nil, ErrMsg("syscall.Sycall6() failed to informal kernel about Umem Fill ring size", errno)
	}

	rc, _, errno = syscall.Syscall6(
		unix.SYS_SETSOCKOPT,
		uintptr(xsk.fd),
		uintptr(unix.SOL_XDP),
		uintptr(unix.XDP_UMEM_COMPLETION_RING),
		uintptr(unsafe.Pointer(&xsk.Opts.RingSize)),
		uintptr(unsafe.Sizeof(xsk.Opts.RingSize)),
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

	xsk.rx, err = NewRing(xsk.fd, uint64(xsk.Opts.RingSize), unix.XDP_PGOFF_RX_RING, offs.Rx)
	if err != nil {
		return nil, ErrMsg("NewRing() failed to creating rx ring", err)
	}

	xsk.fill, err = NewURing(xsk.fd, uint64(xsk.Opts.RingSize), unix.XDP_UMEM_PGOFF_FILL_RING, offs.Fr)
	if err != nil {
		return nil, ErrMsg("NewURing() failed to creating fr ring", err)
	}

	xsk.comp, err = NewURing(xsk.fd, uint64(xsk.Opts.RingSize), unix.XDP_UMEM_PGOFF_COMPLETION_RING, offs.Cr)
	if err != nil {
		return nil, ErrMsg("NewURing() failed to creating cr ring", err)
	}

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

// Socket closes all the umem and rings
// As on now Close assumes socket is using all the rings,
// which may not be the case so I need to add a nil checker
func (s *Socket) Close() error {
	if err := s.rx.Close(); err != nil {
		return ErrMsg("socket failed to close Rx ring", err)
	}

	// if err := s.tx.Close(); err != nil {
	// 	return ErrMsg("socket failed to close Tx ring", err)
	// }

	if err := s.fill.Close(); err != nil {
		return ErrMsg("socket failed to close Fill ring", err)
	}
	if err := s.comp.Close(); err != nil {
		return ErrMsg("socket failed to close Comp ring", err)
	}

	return ErrMsg("socket failed to close UMem region", s.umem.Close())
}

func ErrMsg(msg string, err error) error {
	if err == nil {
		return nil
	}
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
