package system

import (
	"fmt"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	revealShell              = windows.NewLazySystemDLL("shell32.dll")
	parseDisplayName         = revealShell.NewProc("SHParseDisplayName")
	openFolderAndSelectItems = revealShell.NewProc("SHOpenFolderAndSelectItems")
	findLastID               = revealShell.NewProc("ILFindLastID")
)

func revealFile(path string) error {
	// Shell APIs require an initialized COM apartment on the calling thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != syscall.Errno(1) {
		return fmt.Errorf("initialize file manager: %w", err)
	}
	defer windows.CoUninitialize()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	// Go's temporary directory (and some user profiles) can use DOS 8.3
	// aliases. Explorer's shell namespace uses the long path for selection.
	buffer := make([]uint16, 32768)
	if _, err := windows.GetLongPathName(name, &buffer[0], uint32(len(buffer))); err != nil {
		return fmt.Errorf("resolve recording path: %w", err)
	}
	name = &buffer[0]
	var pidl unsafe.Pointer
	hr, _, _ := parseDisplayName.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&pidl)), 0, 0)
	if int32(hr) < 0 {
		return fmt.Errorf("locate recording in Explorer: HRESULT 0x%08x", uint32(hr))
	}
	defer windows.CoTaskMemFree(pidl)
	// Pass the parent and an explicit child item, so selection also works on
	// Explorer versions that only navigate when given a zero-length item list.
	parentName, err := windows.UTF16PtrFromString(filepath.Dir(windows.UTF16ToString(buffer)))
	if err != nil {
		return err
	}
	var parent unsafe.Pointer
	hr, _, _ = parseDisplayName.Call(uintptr(unsafe.Pointer(parentName)), 0, uintptr(unsafe.Pointer(&parent)), 0, 0)
	if int32(hr) < 0 {
		return fmt.Errorf("locate Explorer parent folder: HRESULT 0x%08x", uint32(hr))
	}
	defer windows.CoTaskMemFree(parent)
	child, _, _ := findLastID.Call(uintptr(pidl))
	hr, _, _ = openFolderAndSelectItems.Call(uintptr(parent), 1, uintptr(unsafe.Pointer(&child)), 0)
	if int32(hr) < 0 {
		return fmt.Errorf("show recording in Explorer: HRESULT 0x%08x", uint32(hr))
	}
	return nil
}
