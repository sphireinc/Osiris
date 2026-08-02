//go:build !windows

package osiris

import "os"

func stdOSGetuid() int               { return os.Getuid() }
func stdOSGeteuid() int              { return os.Geteuid() }
func stdOSGetgid() int               { return os.Getgid() }
func stdOSGetegid() int              { return os.Getegid() }
func stdOSGetgroups() ([]int, error) { return os.Getgroups() }

func stdOSChown(path string, uid, gid int, link bool) error {
	if link {
		return os.Lchown(path, uid, gid)
	}
	return os.Chown(path, uid, gid)
}
