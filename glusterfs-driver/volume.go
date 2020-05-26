package glusterfsdriver

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"os/exec"
)

type glusterfsVolume struct {
	Servers    string
	VolumeName string
	Options    map[string]string

	Mountpoint  string
	connections int
}

func (gv *glusterfsVolume) mount() error {
	cmd := gv.getMountCmd()
	logrus.Debug(cmd.Args)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mount command execute failed: %v (%s)", err, output)
	}
	return nil
}

func (gv *glusterfsVolume) getMountCmd() *exec.Cmd {
	volumefile := fmt.Sprintf("%v:/%v", gv.Servers, gv.VolumeName)
	cmd := exec.Command(
		"mount", "-t", "glusterfs", volumefile, gv.Mountpoint,
		"-o", "log-file=/run/docker/plugins/init-stdout")

	for key, val := range gv.Options {
		if val != "" {
			cmd.Args = append(cmd.Args, "-o", key+"="+val)
		} else {
			cmd.Args = append(cmd.Args, "-o", key)
		}
	}

	return cmd
}

func (gv *glusterfsVolume) unmount() error {
	cmd := exec.Command("umount", gv.Mountpoint)
	logrus.Debug(cmd.Args)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("umount command execute failed: %v (%s)", err, output)
	}
	return nil
}
