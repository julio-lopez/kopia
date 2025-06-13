//go:build windows
// +build windows

// Package stat provides a cross-platform abstraction for
// common stat commands.
package stat

import (
	"math"
	"runtime"
	"unsafe"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// GetFileAllocSize gets the space allocated on disk for the file
// 'fname' in bytes.
//
//nolint:revive
func GetFileAllocSize(fname string) (uint64, error) {
	// Convert the file name to a UTF-16 pointer
	namePtr, err := windows.UTF16PtrFromString(fname)
	if err != nil {
		return 0, errors.Wrap(err, "failed to convert file name to UTF-16")
	}

	// Open the file
	handle, err := windows.CreateFile(
		namePtr,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return 0, errors.Wrap(err, "failed to open file")
	}
	defer windows.CloseHandle(handle)

	// Get file information
	var fileInfo windows.ByHandleFileInformation
	err = windows.GetFileInformationByHandle(handle, &fileInfo)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get file information")
	}

	// Calculate the allocated size
	allocSize := uint64(fileInfo.FileSizeHigh)<<32 + uint64(fileInfo.FileSizeLow)
	return allocSize, nil
}

// GetBlockSize gets the disk block size of the underlying system.
//
//nolint:revive
func GetBlockSize(path string) (uint64, error) {
	kernel32 := windows.NewLazyDLL("kernel32.dll")
	getDiskFreeSpace := kernel32.NewProc("GetDiskFreeSpaceW")

	var sectorsPerCluster, bytesPerSector, freeClusters, totalClusters uint32

	ret, _, err := getDiskFreeSpace.Call(
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(path))),
		uintptr(unsafe.Pointer(&sectorsPerCluster)),
		uintptr(unsafe.Pointer(&bytesPerSector)),
		uintptr(unsafe.Pointer(&freeClusters)),
		uintptr(unsafe.Pointer(&totalClusters)),
	)
	if ret == 0 {
		return math.MaxUint64, errors.Wrapf(err, "Error while getting block size for %v", runtime.GOOS)
	}

	// Calculate the block size as sectors per cluster * bytes per sector
	return uint64(sectorsPerCluster) * uint64(bytesPerSector), nil
}
