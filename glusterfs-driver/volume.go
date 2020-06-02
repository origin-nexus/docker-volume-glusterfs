package glusterfsdriver

import (
	"bufio"
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"strings"
)

type glusterfsVolume struct {
	Servers    string
	VolumeName string
	Options    map[string]string

	Mountpoint string
	mounted    bool

	executeCommand func(string, ...string) ([]byte, error)
}

func (gv *glusterfsVolume) Mount() error {
	if gv.isMounted() {
		return nil
	}

	args := gv.getMountArgs()
	logrus.Debug(args)

	output, err := gv.executeCommand("mount", args...)
	if err != nil {
		return fmt.Errorf("mount command execute failed: %v (%s)", err, output)
	}
	gv.mounted = true
	return nil
}

func (gv *glusterfsVolume) getMountArgs() []string {
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

func (gv *glusterfsVolume) Unmount() error {
	if !gv.isMounted() {
		logrus.Debugf("'%v' not mounted, so not unmounting", gv.VolumeName)
		return nil
	}

	output, err := gv.executeCommand("umount", gv.Mountpoint)
	if err != nil {
		return fmt.Errorf("umount command execute failed: %v (%s)", err, output)
	}
	gv.mounted = false
	return nil
}

func (gv *glusterfsVolume) isMounted() bool {
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
