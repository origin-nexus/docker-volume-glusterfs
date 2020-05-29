package glusterfsdriver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sync"

	"github.com/docker/go-plugins-helpers/volume"
	"github.com/sirupsen/logrus"
)

type DockerVolume struct {
	GlusterVolumeId string
	Mountpoint      string
}

type State struct {
	DockerVolumes  map[string]*DockerVolume
	GlusterVolumes map[string]*glusterfsVolume
}

type Driver struct {
	sync.Mutex

	root      string
	statePath string

	dedicatedMounts bool

	servers    string
	volumeName string
	options    map[string]string

	state State
}

func NewDriver(root string, statePath string, options map[string]string) *Driver {
	logrus.WithField("method", "new driver").Debug(root)

	servers, _ := options["servers"]
	delete(options, "servers")

	volumeName, _ := options["volume-name"]
	delete(options, "volume-name")

	_, dedicatedMounts := options["dedicated-mount"]
	delete(options, "dedicated-mount")

	return &Driver{
		root:            root,
		statePath:       statePath,
		servers:         servers,
		volumeName:      volumeName,
		dedicatedMounts: dedicatedMounts,
		options:         options,
		state: State{
			DockerVolumes:  map[string]*DockerVolume{},
			GlusterVolumes: map[string]*glusterfsVolume{},
		},
	}
}

func (d *Driver) Capabilities() *volume.CapabilitiesResponse {
	logrus.WithField("method", "capabilities").Debugf("")

	return &volume.CapabilitiesResponse{Capabilities: volume.Capability{Scope: "local"}}
}

func CheckOption(key, val string) error {
	switch key {
	case "backup-volfile-server":
		fallthrough
	case "backup-volfile-servers":
		return fmt.Errorf("'%v' option not supported", key)
	case "log-file":
		return fmt.Errorf("'%v' option not supported, logs are redirected to managed plugin stdout", key)
	case "servers":
		fallthrough
	case "volume-name":
		return fmt.Errorf("'%v' option not supported in options", key)
	}
	return nil
}

func (d *Driver) GetOrCreateGlusterVolume(servers string, volumeName string, options map[string]string) (string, error) {

	gv := &glusterfsVolume{
		Servers:    servers,
		VolumeName: volumeName,
		Options:    d.options,
	}

	dedicatedMount := d.dedicatedMounts

	for key, val := range options {
		switch key {
		case "dedicated-mount":
			dedicatedMount = true
		default:
			if err := CheckOption(key, val); err != nil {
				return "", err
			}
			if len(d.options) != 0 {
				return "", errors.New("Options already set by driver, can not override.")
			}
			gv.Options[key] = val
		}
	}

	if gv.Servers == "" {
		return "", errors.New("'servers' option required")
	}

	id := ""
	if dedicatedMount {
		i := 1
		for {
			id = filepath.Join("_dedicated", gv.Servers, gv.VolumeName, string(i))
			if _, exists := d.state.GlusterVolumes[id]; !exists {
				break
			}
			i++
		}
	} else {
		id = filepath.Join(gv.Servers, gv.VolumeName)
	}
	gv.Mountpoint = filepath.Join(d.root, id)
	if existingVolume, ok := d.state.GlusterVolumes[id]; ok {
		if !reflect.DeepEqual(gv.Options, existingVolume.Options) {
			var volumes []string
			for name, v := range d.state.DockerVolumes {
				if v.GlusterVolumeId == id {
					volumes = append(volumes, name)
				}
			}
			return "", fmt.Errorf(
				"%#v options differ from already created volumes %#v with options %#v"+
					" use 'dedicated-mount' option to not reuse existing mounts.",
				gv.Options, volumes, existingVolume.Options)
		}
		gv = existingVolume
	} else {
		d.state.GlusterVolumes[id] = gv
	}

	return id, nil
}

func (d *Driver) Create(r *volume.CreateRequest) error {
	logrus.WithField("method", "create").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	servers := d.servers
	volumeName := d.volumeName

	if servers == "" {
		servers = r.Options["servers"]
		delete(r.Options, "servers")
	} else if _, exists := r.Options["servers"]; exists {
		return errors.New("'servers' option already set by driver, can not override.")
	}

	if servers == "" {
		return errors.New("'servers' option required")
	}

	if volumeName == "" {
		volumeName = r.Options["volume-name"]
		delete(r.Options, "volume-name")
	} else if _, exists := r.Options["volume-name"]; exists {
		return errors.New("'volume-name' option already set by driver, can not override.")
	}

	subdirMount := ""
	if volumeName == "" {
		volumeName = r.Name
	} else {
		subdirMount = r.Name
	}

	id, err := d.GetOrCreateGlusterVolume(servers, volumeName, r.Options)
	if err != nil {
		return err
	}
	gv := d.state.GlusterVolumes[id]

	mountpoint := gv.Mountpoint
	if subdirMount != "" {
		mountpoint = filepath.Join(mountpoint, subdirMount)
	}

	d.state.DockerVolumes[r.Name] = &DockerVolume{GlusterVolumeId: id, Mountpoint: mountpoint}

	d.saveState()

	return nil
}

func (d *Driver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	logrus.WithField("method", "get").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.state.DockerVolumes[r.Name]
	if !ok {
		return &volume.GetResponse{}, fmt.Errorf("volume %s not found", r.Name)
	}

	return &volume.GetResponse{Volume: &volume.Volume{Name: r.Name, Mountpoint: v.Mountpoint}}, nil
}

func (d *Driver) List() (*volume.ListResponse, error) {
	logrus.WithField("method", "list").Debugf("")

	d.Lock()
	defer d.Unlock()

	var vols []*volume.Volume
	for name, v := range d.state.DockerVolumes {
		vols = append(vols, &volume.Volume{Name: name, Mountpoint: v.Mountpoint})
	}
	return &volume.ListResponse{Volumes: vols}, nil
}

func (d *Driver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	logrus.WithField("method", "path").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.state.DockerVolumes[r.Name]
	if !ok {
		return &volume.PathResponse{}, fmt.Errorf("volume %s not found", r.Name)
	}

	return &volume.PathResponse{Mountpoint: v.Mountpoint}, nil
}

func (d *Driver) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	logrus.WithField("method", "mount").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.state.DockerVolumes[r.Name]
	if !ok {
		return &volume.MountResponse{}, fmt.Errorf("volume %s not found", r.Name)
	}
	logrus.WithField("method", "mount").Debugf("found volume %#v", v)

	gv := d.state.GlusterVolumes[v.GlusterVolumeId]

	logrus.WithField("method", "mount").Debugf("found gluster volume %#v", gv)

	if gv.connections == 0 {
		fi, err := os.Lstat(gv.Mountpoint)
		if os.IsNotExist(err) {
			if err := os.MkdirAll(gv.Mountpoint, 0755); err != nil {
				return &volume.MountResponse{}, err
			}
		} else if err != nil {
			return &volume.MountResponse{}, err
		}

		if fi != nil && !fi.IsDir() {
			return &volume.MountResponse{},
				fmt.Errorf("%v already exist and it's not a directory", gv.Mountpoint)
		}

		if err := gv.mount(); err != nil {
			return &volume.MountResponse{}, err
		}
	}

	// Create subdirectory if required
	if v.Mountpoint != gv.Mountpoint {
		fi, err := os.Lstat(v.Mountpoint)
		if os.IsNotExist(err) {
			if err := os.MkdirAll(v.Mountpoint, 0755); err != nil {
				if gv.connections == 0 {
					gv.unmount()
				}
				return &volume.MountResponse{}, err
			}
		} else if err != nil {
			if gv.connections == 0 {
				gv.unmount()
			}
			return &volume.MountResponse{}, err
		}

		if fi != nil && !fi.IsDir() {
			if gv.connections == 0 {
				gv.unmount()
			}
			return &volume.MountResponse{},
				fmt.Errorf("%v already exist and it's not a directory", v.Mountpoint)
		}
	}

	gv.connections++
	d.saveState()

	return &volume.MountResponse{Mountpoint: v.Mountpoint}, nil
}

func (d *Driver) Unmount(r *volume.UnmountRequest) error {
	logrus.WithField("method", "unmount").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.state.DockerVolumes[r.Name]
	if !ok {
		return fmt.Errorf("volume %s not found", r.Name)
	}

	gv := d.state.GlusterVolumes[v.GlusterVolumeId]

	if gv.connections > 0 {
		gv.connections--
		d.saveState()
	}

	if gv.connections == 0 {
		if err := gv.unmount(); err != nil {
			return err
		}
	}

	return nil
}

func (d *Driver) Remove(r *volume.RemoveRequest) error {
	logrus.WithField("method", "remove").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.state.DockerVolumes[r.Name]
	if !ok {
		return fmt.Errorf("volume %s not found", r.Name)
	}

	gvId := v.GlusterVolumeId
	delete(d.state.DockerVolumes, r.Name)

	gvUsed := false
	for _, v := range d.state.DockerVolumes {
		if v.GlusterVolumeId == gvId {
			gvUsed = true
			break
		}
	}
	if !gvUsed {
		delete(d.state.GlusterVolumes, gvId)
	}

	d.saveState()

	return nil
}
func (d *Driver) LoadState() error {
	logrus.WithField("method", "LoadState").Debugf("loading state from '%v'", d.statePath)

	data, err := ioutil.ReadFile(d.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			logrus.WithField("statePath", d.statePath).Debug("no state found")
		} else {
			return err
		}
	} else {
		if err := json.Unmarshal(data, &d.state); err != nil {
			return err
		}
	}

	return nil
}

func (d *Driver) saveState() {
	logrus.WithField("method", "saveState").Debugf(
		"saving state %#v to '%v'", d.state, d.statePath)
	data, err := json.Marshal(d.state)
	if err != nil {
		logrus.WithField("statePath", d.statePath).Error(err)
		return
	}

	if err := ioutil.WriteFile(d.statePath, data, 0644); err != nil {
		logrus.WithField("savestate", d.statePath).Error(err)
	}

}

func (d *Driver) GetOptions() map[string]string {
	return d.options
}
func (d *Driver) DedicatesMounts() bool {
	return d.dedicatedMounts
}
