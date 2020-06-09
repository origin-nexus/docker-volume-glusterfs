package glusterfsvolume

import (
	"bufio"
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"strings"
)

type Volume struct {
	Servers    string
	VolumeName string
	Options    map[string]string

	Mountpoint string
}

var ExecuteCommand = func(cmd string, args ...string) ([]byte, error) {
	return exec.Command(cmd, args...).CombinedOutput()
}

func (gv *Volume) Mount() error {
	if gv.isMounted() {
		return nil
	}

	if err := gv.createMountpoint(); err != nil {
		return fmt.Errorf("error creating mount point: %v)", err)
	}

	args := gv.getMountArgs()
	logrus.Debug(args)

	if output, err := ExecuteCommand("mount", args...); err != nil {
		return fmt.Errorf("mount command execute failed: %v (%s)", err, output)
	}
	return nil
}

func (gv *Volume) getMountArgs() []string {
	volumefile := fmt.Sprintf("%v:/%v", gv.Servers, gv.VolumeName)
	args := []string{
		"-t", "glusterfs", volumefile, gv.Mountpoint,
		"-o", "log-file=/run/docker/plugins/init-stdout"}

	for key, val := range gv.Options {
		if val != "" {
			args = append(args, "-o", key+"="+val)
		} else {
			args = append(args, "-o", key)
		}
	}

	return args
}

func (gv *Volume) Unmount() error {
	if !InMtab(gv.Mountpoint) {
		logrus.Debugf("'%v' not mounted, so not unmounting", gv.VolumeName)
		return nil
	}

	output, err := ExecuteCommand("umount", gv.Mountpoint)
	if err != nil {
		return fmt.Errorf("umount command execute failed: %v (%s)", err, output)
	}
	return nil
}

func (gv *Volume) isMounted() bool {
	if !InMtab(gv.Mountpoint) {
		return false
	}

	_, err := os.Stat(gv.Mountpoint)
	if err == nil {
		return true
	}
	// force unmount as it seems stale.
	gv.Unmount()
	return false
}

func InMtab(mountpoint string) bool {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		logrus.Errorf("Failed to open '/proc/mounts': %v", err)
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		mount := strings.Split(scanner.Text(), " ")
		if mount[1] == mountpoint {
			return true
		}
	}
	return false
}

func (gv *Volume) createMountpoint() error {
	fi, err := os.Lstat(gv.Mountpoint)

	if os.IsNotExist(err) {
		if err := os.MkdirAll(gv.Mountpoint, 0755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if fi != nil && !fi.IsDir() {
		return fmt.Errorf("%v already exist and it's not a directory", gv.Mountpoint)
	}

	return nil
}
