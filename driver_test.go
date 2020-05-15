package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/docker/go-plugins-helpers/volume"
)

func TestUnsupportedOptions(t *testing.T) {
	unsupportedOptions := []string{"backup-volfile-server", "backup-volfile-servers", "log-file"}

	d := glusterfsDriver{
		state: State{
			Volumes:        map[string]*Volume{},
			GlusterVolumes: map[string]*glusterfsVolume{},
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
	d := glusterfsDriver{
		servers: "server1,server2",
		state: State{
			Volumes:        map[string]*Volume{},
			GlusterVolumes: map[string]*glusterfsVolume{},
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
	d := glusterfsDriver{
		servers:    "server1,server2",
		volumeName: "myvol",
		state: State{
			Volumes:        map[string]*Volume{},
			GlusterVolumes: map[string]*glusterfsVolume{},
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

func TestSubDirMount(t *testing.T) {
	d := glusterfsDriver{
		root:       "/mnt",
		servers:    "server1,server2",
		volumeName: "myvol",
		state: State{
			Volumes:        map[string]*Volume{},
			GlusterVolumes: map[string]*glusterfsVolume{},
		},
	}
	r := &volume.CreateRequest{
		Name: "test",
	}

	if err := d.Create(r); err != nil {
		t.Errorf("Unexpected error '%v'", err)
	}

	volume := d.state.Volumes["test"]
	gv := d.state.GlusterVolumes[volume.GlusterVolumeId]
	if volume.Mountpoint != filepath.Join(gv.Mountpoint, "/test") {
		t.Errorf("Unexpected mount point '%v'", volume.Mountpoint)
	}
}

func TestNoSubDirMount(t *testing.T) {
	d := glusterfsDriver{
		root:    "/mnt",
		servers: "server1,server2",
		state: State{
			Volumes:        map[string]*Volume{},
			GlusterVolumes: map[string]*glusterfsVolume{},
		},
	}
	r := &volume.CreateRequest{
		Name: "test",
	}

	if err := d.Create(r); err != nil {
		t.Errorf("Unexpected error '%v'", err)
	}

	volume := d.state.Volumes["test"]
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

	d := glusterfsDriver{
		root:       tmpDir,
		statePath:  statePath,
		servers:    "server1,server2",
		volumeName: "myvol",
		state: State{
			Volumes:        map[string]*Volume{},
			GlusterVolumes: map[string]*glusterfsVolume{},
		},
	}

	r := &volume.CreateRequest{
		Name: "test",
	}

	if err := d.Create(r); err != nil {
		t.Errorf("Unexpected error '%v'", err)
	}

	d.saveState()

	d2 := glusterfsDriver{statePath: statePath}
	d2.loadState()

	if !reflect.DeepEqual(d.state, d2.state) {
		t.Errorf(
			"Loaded state '%#v' differs from original state '%#v'",
			d2.state, d.state)
	}
}
