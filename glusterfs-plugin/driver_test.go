package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/docker/go-plugins-helpers/volume"

	"github.com/origin-nexus/docker-volume-glusterfs/glusterfs-volume"
)

func TestUnsupportedOptions(t *testing.T) {
	unsupportedOptions := []string{"backup-volfile-server", "backup-volfile-servers", "log-file"}

	d := Driver{
		state: State{
			DockerVolumes:  map[string]*DockerVolume{},
			GlusterVolumes: map[string]*glusterfsvolume.GlusterfsVolume{},
		},
	}
	for _, option := range unsupportedOptions {
		r := &volume.CreateRequest{
			Name: "test",
			Options: map[string]string{
				option: "whatever",
			},
		}

		if d.Create(r) == nil {
			t.Errorf("Unsupported option '%v' should return error", option)
		}
	}
}

func TestNoServerOverride(t *testing.T) {
	d := Driver{
		glusterConfig: glusterfsvolume.Config{
			Servers: "server1,server2",
		},
		state: State{
			DockerVolumes:  map[string]*DockerVolume{},
			GlusterVolumes: map[string]*glusterfsvolume.GlusterfsVolume{},
		},
	}
	r := &volume.CreateRequest{
		Name: "test",
		Options: map[string]string{
			"servers": "whatever",
		},
	}

	if d.Create(r) == nil {
		t.Error("Overriding 'servers' option should return error")
	}
}

func TestNoVolumeOverride(t *testing.T) {
	d := Driver{
		glusterConfig: glusterfsvolume.Config{
			Servers:    "server1,server2",
			VolumeName: "myvol",
		},
		state: State{
			DockerVolumes:  map[string]*DockerVolume{},
			GlusterVolumes: map[string]*glusterfsvolume.GlusterfsVolume{},
		},
	}
	r := &volume.CreateRequest{
		Name: "test",
		Options: map[string]string{
			"servers":     "whatever",
			"volume-name": "other-volume",
		},
	}

	if d.Create(r) == nil {
		t.Error("Overriding 'volume-name' option should return error")
	}
}

type executor struct {
	cmd  string
	args []string
}

func (e *executor) exec(cmd string, args ...string) ([]byte, error) {
	e.cmd = cmd
	e.args = args

	return []byte{}, nil
}

func TestSubDirMount(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "glusterfs-plugin-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	e := executor{}
	glusterfsvolume.ExecuteCommand = e.exec

	d := Driver{
		root: tmpDir,
		glusterConfig: glusterfsvolume.Config{
			Servers:    "server1,server2",
			VolumeName: "myvol",
		},
		state: State{
			DockerVolumes:  map[string]*DockerVolume{},
			GlusterVolumes: map[string]*glusterfsvolume.GlusterfsVolume{},
		},
	}
	r := &volume.CreateRequest{
		Name: "test",
	}

	if err := d.Create(r); err != nil {
		t.Errorf("Unexpected error '%v'", err)
		return
	}
	if e.cmd != "mount" {
		t.Errorf("Unexpected command '%v'", e.cmd)
	}

	volume := d.state.DockerVolumes["test"]
	gv := d.state.GlusterVolumes[volume.GlusterVolumeId]
	if volume.Mountpoint != filepath.Join(gv.Mountpoint, "/test") {
		t.Errorf("Unexpected mount point '%v'", volume.Mountpoint)
	}
}

func TestNoSubDirMount(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "glusterfs-plugin-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	d := Driver{
		root: tmpDir,
		glusterConfig: glusterfsvolume.Config{
			Servers: "server1,server2",
		},
		state: State{
			DockerVolumes:  map[string]*DockerVolume{},
			GlusterVolumes: map[string]*glusterfsvolume.GlusterfsVolume{},
		},
	}
	r := &volume.CreateRequest{
		Name: "test",
	}

	if err := d.Create(r); err != nil {
		t.Errorf("Unexpected error '%v'", err)
		return
	}

	volume := d.state.DockerVolumes["test"]
	gv := d.state.GlusterVolumes[volume.GlusterVolumeId]
	if volume.Mountpoint != gv.Mountpoint {
		t.Errorf("Unexpected mount point '%v'", volume.Mountpoint)
	}
}

func TestStateSaveAndLoad(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "glusterfs-plugin-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "test-state.json")

	d := Driver{
		root:      tmpDir,
		statePath: statePath,
		glusterConfig: glusterfsvolume.Config{
			Servers:    "server1,server2",
			VolumeName: "myvol",
		},
		state: State{
			DockerVolumes:  map[string]*DockerVolume{},
			GlusterVolumes: map[string]*glusterfsvolume.GlusterfsVolume{},
		},
	}

	r := &volume.CreateRequest{
		Name: "test",
	}

	if err := d.Create(r); err != nil {
		t.Errorf("Unexpected error '%v'", err)
		return
	}

	// unmount volumes so that states can be compared after load (mount not exported)
	for _, gv := range d.state.GlusterVolumes {
		gv.Unmount()
	}

	d.saveState()

	d2 := Driver{statePath: statePath}
	d2.LoadState()

	if !reflect.DeepEqual(d.state, d2.state) {
		t.Errorf(
			"Loaded state\n%#v\n differs from original state\n%#v", d2.state, d.state)
	}
}
