// lpdiag is a diagnostic for launchpad.go's substream handling: it opens the
// Launchpad's rawmidi device the same way ConnectLaunchpad does
// (prefer-subdevice ioctl, then open) and then asks the kernel, via
// SNDRV_RAWMIDI_IOCTL_INFO, which substream the resulting fd actually bound
// to for each direction. Input and output can differ, which a plain "it
// connected with no error" check cannot reveal.
//
// Usage: lpdiag <alsa-card-index>   (see /proc/asound/cards)
package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	sndrvCtlIoctlRawmidiPreferSubdevice = 0x40045542
	sndrvRawmidiIoctlInfo               = 0x810c5701
	streamOutput                        = 0
	streamInput                         = 1
)

type rawmidiInfo struct {
	Device          uint32
	Subdevice       uint32
	Stream          int32
	Card            int32
	Flags           uint32
	ID              [64]byte
	Name            [80]byte
	Subname         [32]byte
	SubdevicesCount uint32
	SubdevicesAvail uint32
	TiedDevice      int32
	Reserved        [60]byte
}

func main() {
	const wantSubdevice = 1
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: lpdiag <alsa-card-index>   (see /proc/asound/cards)")
		os.Exit(2)
	}
	cardIndex, err := strconv.Atoi(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "card index:", err)
		os.Exit(2)
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ctl, err := os.OpenFile(fmt.Sprintf("/dev/snd/controlC%d", cardIndex), os.O_RDWR, 0)
	if err != nil {
		fmt.Println("open control:", err)
		os.Exit(1)
	}
	if err := unix.IoctlSetPointerInt(int(ctl.Fd()), sndrvCtlIoctlRawmidiPreferSubdevice, wantSubdevice); err != nil {
		fmt.Println("prefer subdevice:", err)
		os.Exit(1)
	}
	// The preference is scoped to the control fd staying open, so close it
	// only after the rawmidi open.
	defer ctl.Close()

	f, err := os.OpenFile(fmt.Sprintf("/dev/snd/midiC%dD0", cardIndex), os.O_RDWR, 0)
	if err != nil {
		fmt.Println("open rawmidi:", err)
		os.Exit(1)
	}
	defer f.Close()

	for _, s := range []struct {
		name   string
		stream int32
	}{{"OUTPUT", streamOutput}, {"INPUT", streamInput}} {
		info := rawmidiInfo{Stream: s.stream}
		_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), uintptr(sndrvRawmidiIoctlInfo), uintptr(unsafe.Pointer(&info)))
		if errno != 0 {
			fmt.Printf("%s: ioctl error: %v\n", s.name, errno)
			continue
		}
		fmt.Printf("%s: bound to subdevice %d (name %q, subname %q)\n",
			s.name, info.Subdevice, cString(info.Name[:]), cString(info.Subname[:]))
	}
}

func cString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
