package mod_tcp_keepalive

import (
	"os"
	"syscall"
)

// from netinet/tcp.h (OS X 10.9.4)
const (
	_TCP_KEEPINTVL = 0x101 /* interval between keepalives */
	_TCP_KEEPCNT   = 0x102 /* number of keepalives before close */
)

func setIdle(fd int, secs int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_KEEPALIVE, secs))
}

func setCount(fd int, n int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, _TCP_KEEPCNT, n))
}

func setInterval(fd int, secs int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, _TCP_KEEPINTVL, secs))
}
