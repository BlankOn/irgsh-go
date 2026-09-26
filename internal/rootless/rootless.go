package rootless

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func HasSubordinateRange(contents []byte, username string) bool {
	for _, line := range strings.Split(string(contents), "\n") {
		fields := strings.Split(line, ":")
		if fields[0] != username {
			continue
		}
		if len(fields) != 3 {
			return false
		}
		start, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return false
		}
		count, err := strconv.ParseUint(fields[2], 10, 64)
		const maxID = uint64(1<<32 - 2)
		return err == nil && count >= 65536 && start <= maxID && count-1 <= maxID-start
	}
	return false
}

func CheckMappingHelper(name string, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 {
		return fmt.Errorf("%s mapping helper must be owned by root; restore permissions with the uidmap package", name)
	}
	if info.Mode()&os.ModeSetuid == 0 {
		return fmt.Errorf("%s mapping helper needs the setuid bit; restore permissions with the uidmap package", name)
	}
	return nil
}
