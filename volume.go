package main

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"os/exec"
)

type glusterfsVolume struct {
	servers    string
	volumeName string
	options    map[string]string

	mountpoint  string
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
	volumefile := fmt.Sprintf("%v:/%v", gv.servers, gv.volumeName)
	cmd := exec.Command(
		"mount", "-t", "glusterfs", volumefile, gv.mountpoint,
		"-o", "log-file=/run/docker/plugins/init-stdout")

	for key, val := range gv.options {
		if val != "" {
			cmd.Args = append(cmd.Args, "-o", key+"="+val)
		} else {
			cmd.Args = append(cmd.Args, "-o", key)
		}
	}

	return cmd
}

func (gv *glusterfsVolume) unmount() error {
	cmd := exec.Command("umount", gv.mountpoint)
	logrus.Debug(cmd.Args)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("umount command execute failed: %v (%s)", err, output)
	}
	return nil
}
