package main

import (
	"reflect"
	"testing"
)

func TestGetMountCmd(t *testing.T) {
	cases := []struct {
		gv   glusterfsVolume
		args []string
	}{
		{
			glusterfsVolume{
				servers:    "server1",
				volumeName: "volume",
				mountpoint: "/mnt",
			},
			[]string{"mount", "-t", "glusterfs", "server1:/volume", "/mnt",
				"-o", "log-file=/run/docker/plugins/init-stdout"},
		},
		{
			glusterfsVolume{
				servers:    "server1",
				volumeName: "volume",
				mountpoint: "/mnt",
				options: map[string]string{
					"option1": "",
				},
			},
			[]string{"mount", "-t", "glusterfs", "server1:/volume", "/mnt",
				"-o", "log-file=/run/docker/plugins/init-stdout", "-o", "option1"},
		},
		{
			glusterfsVolume{
				servers:    "server1",
				volumeName: "volume",
				mountpoint: "/mnt",
				options: map[string]string{
					"option": "value",
				},
			},
			[]string{"mount", "-t", "glusterfs", "server1:/volume", "/mnt",
				"-o", "log-file=/run/docker/plugins/init-stdout", "-o", "option=value"},
		},
	}

	for _, c := range cases {
		if !reflect.DeepEqual(c.gv.getMountCmd().Args, c.args) {
			t.Errorf(
				"incorrect command args\n %v\n expected\n %v",
				c.gv.getMountCmd().Args, c.args)
		}
	}
}
