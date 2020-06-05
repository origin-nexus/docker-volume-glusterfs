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
	mounted    bool
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
	gv.mounted = true
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
	if !gv.isMounted() {
		logrus.Debugf("'%v' not mounted, so not unmounting", gv.VolumeName)
		return nil
	}

	output, err := ExecuteCommand("umount", gv.Mountpoint)
	if err != nil {
		return fmt.Errorf("umount command execute failed: %v (%s)", err, output)
	}
	gv.mounted = false
	return nil
}

func (gv *Volume) isMounted() bool {
	if !gv.mounted {
		// check if already mounted
		f, err := os.Open("/proc/mounts")
		if err != nil {
			logrus.Errorf("Failed to open '/proc/mounts': %v", err)
			return gv.mounted
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			mount := strings.Split(scanner.Text(), " ")
			if mount[1] == gv.Mountpoint {
				gv.mounted = true
				break
			}
		}
	}

	return gv.mounted
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
