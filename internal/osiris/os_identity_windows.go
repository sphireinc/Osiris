//go:build windows

package osiris

import "os"

func stdOSGetuid() int                        { return -1 }
func stdOSGeteuid() int                       { return -1 }
func stdOSGetgid() int                        { return -1 }
func stdOSGetegid() int                       { return -1 }
func stdOSGetgroups() ([]int, error)          { return nil, os.ErrPermission }
func stdOSChown(string, int, int, bool) error { return os.ErrPermission }
