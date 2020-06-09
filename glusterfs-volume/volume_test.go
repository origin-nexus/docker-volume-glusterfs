package glusterfsvolume

import (
	"reflect"
	"testing"
)

func TestGetMountArgs(t *testing.T) {
	cases := []struct {
		gv   GlusterfsVolume
		args []string
	}{
		{
			GlusterfsVolume{
				Servers:       "server1",
				VolumeName:    "volume",
				MountedVolume: MountedVolume{Mountpoint: "/mnt"},
			},
			[]string{"-t", "glusterfs", "server1:/volume", "/mnt",
				"-o", "log-file=/run/docker/plugins/init-stdout"},
		},
		{
			GlusterfsVolume{
				Servers:       "server1",
				VolumeName:    "volume",
				MountedVolume: MountedVolume{Mountpoint: "/mnt"},
				Options: map[string]string{
					"option1": "",
				},
			},
			[]string{"-t", "glusterfs", "server1:/volume", "/mnt",
				"-o", "log-file=/run/docker/plugins/init-stdout", "-o", "option1"},
		},
		{
			GlusterfsVolume{
				Servers:       "server1",
				VolumeName:    "volume",
				MountedVolume: MountedVolume{Mountpoint: "/mnt"},
				Options: map[string]string{
					"option": "value",
				},
			},
			[]string{"-t", "glusterfs", "server1:/volume", "/mnt",
				"-o", "log-file=/run/docker/plugins/init-stdout", "-o", "option=value"},
		},
	}

	for _, c := range cases {
		if !reflect.DeepEqual(c.gv.getMountArgs(), c.args) {
			t.Errorf(
				"incorrect command args\n %v\n expected\n %v",
				c.gv.getMountArgs(), c.args)
		}
	}
}
