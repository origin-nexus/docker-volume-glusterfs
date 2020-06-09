package glusterfsvolume

import (
	"bufio"
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"strings"
)

type MountedVolume struct {
	Mountpoint string
}

func (mv *MountedVolume) Mount() error {
	panic("Abstract method, provide an implementation.")
}

func (mv *MountedVolume) CreateMountpoint() error {
	fi, err := os.Lstat(mv.Mountpoint)

	if os.IsNotExist(err) {
		if err := os.MkdirAll(mv.Mountpoint, 0755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if fi != nil && !fi.IsDir() {
		return fmt.Errorf("%v already exist and it's not a directory", mv.Mountpoint)
	}

	return nil
}

func (mv *MountedVolume) DeleteMountpoint() error {
	fi, err := os.Lstat(mv.Mountpoint)

	if os.IsNotExist(err) {
		return nil
	}

	if err != nil {
		return err
	}

	if fi != nil && !fi.IsDir() {
		return fmt.Errorf("%v is not a directory !!!", mv.Mountpoint)
	}

	if err := os.Remove(mv.Mountpoint); err != nil {
		return err
	}

	return nil
}

func (mv *MountedVolume) IsMounted() bool {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		logrus.Errorf("Failed to open '/proc/mounts': %v", err)
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		mount := strings.Split(scanner.Text(), " ")
		if mount[1] == mv.Mountpoint {
			return true
		}
	}
	return false
}

func (mv *MountedVolume) Unmount() error {
	if !mv.IsMounted() {
		logrus.Debugf("'%v' not mounted, so not unmounting", mv.Mountpoint)
		return nil
	}

	output, err := ExecuteCommand("umount", mv.Mountpoint)
	if err != nil {
		return fmt.Errorf("umount command execute failed: %v (%s)", err, output)
	}
	return nil
}

type GlusterfsVolume struct {
	Servers    string
	VolumeName string
	Options    map[string]string

	MountedVolume
}

var ExecuteCommand = func(cmd string, args ...string) ([]byte, error) {
	return exec.Command(cmd, args...).CombinedOutput()
}

func (gv *GlusterfsVolume) Mount() error {
	if gv.IsMounted() {
		return nil
	}

	if err := gv.CreateMountpoint(); err != nil {
		return fmt.Errorf("error creating mount point: %v)", err)
	}

	args := gv.getMountArgs()
	logrus.Debug(args)

	if output, err := ExecuteCommand("mount", args...); err != nil {
		return fmt.Errorf("mount command execute failed: %v (%s)", err, output)
	}
	return nil
}

func (gv *GlusterfsVolume) getMountArgs() []string {
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

func (gv *GlusterfsVolume) IsMounted() bool {
	if !gv.MountedVolume.IsMounted() {
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
