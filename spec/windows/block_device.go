// +build windows

package windows

import (
	"syscall"
	"unsafe"

	"github.com/mackerelio/mackerel-agent/logging"
	"github.com/mackerelio/mackerel-agent/util/windows"
)

// BlockDeviceGenerator XXX
type BlockDeviceGenerator struct {
}

// Key XXX
func (g *BlockDeviceGenerator) Key() string {
	return "block_device"
}

var blockDeviceLogger = logging.GetLogger("spec.block_device")

// Generate XXX
func (g *BlockDeviceGenerator) Generate() (interface{}, error) {
	results := make(map[string]map[string]interface{})

	drivebuf := make([]byte, 256)
	windows.GetLogicalDriveStrings.Call(
		uintptr(len(drivebuf)),
		uintptr(unsafe.Pointer(&drivebuf[0])))

	for _, v := range drivebuf {
		if v >= 65 && v <= 90 {
			drive := string(v)
			removable := false
			r, _, _ := windows.GetDriveType.Call(uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(drive + `:\`))))
			if r == windows.DriveRemovable {
				removable = true
			}
			freeBytesAvailable := int64(0)
			totalNumberOfBytes := int64(0)
			totalNumberOfFreeBytes := int64(0)
			r, _, _ = windows.GetDiskFreeSpaceEx.Call(
				uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(drive))),
				uintptr(unsafe.Pointer(&freeBytesAvailable)),
				uintptr(unsafe.Pointer(&totalNumberOfBytes)),
				uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)))
			if r == 0 {
				continue
			}
			results[drive] = map[string]interface{}{
				"size":      totalNumberOfFreeBytes,
				"removable": removable,
			}
		}
	}

	return results, nil
}
edit the error handling to block_device and filesystem. for windows metric.
