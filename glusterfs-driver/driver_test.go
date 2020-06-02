package glusterfsdriver

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

	d := Driver{
		state: State{
			DockerVolumes:  map[string]*DockerVolume{},
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
	d := Driver{
		config: Config{
			servers: "server1,server2",
		},
		state: State{
			DockerVolumes:  map[string]*DockerVolume{},
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
	d := Driver{
		config: Config{
			servers:    "server1,server2",
			volumeName: "myvol",
		},
		state: State{
			DockerVolumes:  map[string]*DockerVolume{},
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

	d := Driver{
		config: Config{
			root:       tmpDir,
			servers:    "server1,server2",
			volumeName: "myvol",
		},
		state: State{
			DockerVolumes:  map[string]*DockerVolume{},
			GlusterVolumes: map[string]*glusterfsVolume{},
		},
		executeCommand: e.exec,
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

	e := executor{}

	d := Driver{
		config: Config{
			root:    tmpDir,
			servers: "server1,server2",
		},
		state: State{
			DockerVolumes:  map[string]*DockerVolume{},
			GlusterVolumes: map[string]*glusterfsVolume{},
		},
		executeCommand: e.exec,
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

	e := executor{}

	d := Driver{
		config: Config{
			root:       tmpDir,
			statePath:  statePath,
			servers:    "server1,server2",
			volumeName: "myvol",
		},
		state: State{
			DockerVolumes:  map[string]*DockerVolume{},
			GlusterVolumes: map[string]*glusterfsVolume{},
		},
		executeCommand: e.exec,
	}

	r := &volume.CreateRequest{
		Name: "test",
	}

	if err := d.Create(r); err != nil {
		t.Errorf("Unexpected error '%v'", err)
		return
	}

	d.saveState()

	d2 := Driver{config: Config{statePath: statePath}, executeCommand: e.exec}
	d2.LoadState()

	for _, gv := range d2.state.GlusterVolumes {
		if gv.executeCommand == nil {
			t.Error("executeCommand on glusterfsVolume has not been provisionned when loading state")
		}
	}

	// same function do not compare well using DeepEqual
	for id, _ := range d.state.GlusterVolumes {
		d.state.GlusterVolumes[id].executeCommand = nil
		d2.state.GlusterVolumes[id].executeCommand = nil
	}

	// ignore mounted
	d2.state.GlusterVolumes["server1,server2/myvol"].mounted = true

	if !reflect.DeepEqual(d.state, d2.state) {
		t.Errorf(
			"Loaded state\n%#v\n differs from original state\n%#v", d2.state, d.state)
	}
}
