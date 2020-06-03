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

type Config struct {
	root      string
	statePath string

	dedicatedMounts bool

	servers    string
	volumeName string
	options    map[string]string
}

type Driver struct {
	sync.Mutex

	config Config
	state  State

	executeCommand func(string, ...string) ([]byte, error)
}

func NewConfig(root string, statePath string, options map[string]string) Config {

	logrus.WithField("method", "NewConfig").Debug(root)

	servers, _ := options["servers"]
	delete(options, "servers")

	volumeName, _ := options["volume-name"]
	delete(options, "volume-name")

	_, dedicatedMounts := options["dedicated-mount"]
	delete(options, "dedicated-mount")

	return Config{
		root:            root,
		statePath:       statePath,
		servers:         servers,
		volumeName:      volumeName,
		dedicatedMounts: dedicatedMounts,
		options:         options,
	}
}

func NewDriver(config Config,
	executeCommand func(string, ...string) ([]byte, error)) *Driver {

	logrus.WithField("method", "new driver").Debug(config)

	return &Driver{
		config: config,
		state: State{
			DockerVolumes:  map[string]*DockerVolume{},
			GlusterVolumes: map[string]*glusterfsVolume{},
		},
		executeCommand: executeCommand,
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
		Servers:        servers,
		VolumeName:     volumeName,
		Options:        d.config.options,
		executeCommand: d.executeCommand,
	}

	dedicatedMount := d.config.dedicatedMounts

	for key, val := range options {
		switch key {
		case "dedicated-mount":
			dedicatedMount = true
		default:
			if err := CheckOption(key, val); err != nil {
				return "", err
			}
			if len(d.config.options) != 0 {
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
	gv.Mountpoint = filepath.Join(d.config.root, id)
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

	servers := d.config.servers
	volumeName := d.config.volumeName

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

	defer d.saveState()
	defer d.deleteUnused(id)

	gv := d.state.GlusterVolumes[id]

	if err := gv.Mount(); err != nil {
		return err
	}

	mountpoint := gv.Mountpoint
	if subdirMount != "" {
		mountpoint = filepath.Join(mountpoint, subdirMount)
		// Create subdirectory if required
		fi, err := os.Lstat(mountpoint)
		if os.IsNotExist(err) {
			if err := os.MkdirAll(mountpoint, 0755); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		if fi != nil && !fi.IsDir() {
			return fmt.Errorf("'%v' already exist in '%v' volume and it's not a directory",
				subdirMount, gv.VolumeName)
		}
	}

	d.state.DockerVolumes[r.Name] = &DockerVolume{GlusterVolumeId: id, Mountpoint: mountpoint}

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

	v, ok := d.state.DockerVolumes[r.Name]
	if !ok {
		return &volume.MountResponse{}, fmt.Errorf("volume %s not found", r.Name)
	}
	logrus.WithField("method", "mount").Debugf("found volume %#v", v)

	d.state.GlusterVolumes[v.GlusterVolumeId].Mount()

	return &volume.MountResponse{Mountpoint: v.Mountpoint}, nil
}

func (d *Driver) Unmount(r *volume.UnmountRequest) error {
	logrus.WithField("method", "unmount").Debugf("%#v", r)

	if _, ok := d.state.DockerVolumes[r.Name]; !ok {
		return fmt.Errorf("volume %s not found", r.Name)
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

	if err := d.deleteUnused(gvId); err != nil {
		return err
	}

	d.saveState()

	return nil
}

func (d *Driver) deleteUnused(gvId string) error {
	gvUsed := false
	for _, v := range d.state.DockerVolumes {
		if v.GlusterVolumeId == gvId {
			gvUsed = true
			break
		}
	}
	if !gvUsed {
		gv := d.state.GlusterVolumes[gvId]
		if err := gv.Unmount(); err != nil {
			return err
		}
		delete(d.state.GlusterVolumes, gvId)
	}

	return nil
}

func (d *Driver) LoadState() error {
	logrus.WithField("method", "LoadState").Debugf("loading state from '%v'", d.config.statePath)

	data, err := ioutil.ReadFile(d.config.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			logrus.WithField("statePath", d.config.statePath).Debug("no state found")
		} else {
			return err
		}
	} else {
		if err := json.Unmarshal(data, &d.state); err != nil {
			return err
		}
	}

	for _, gv := range d.state.GlusterVolumes {
		gv.executeCommand = d.executeCommand
	}

	return nil
}

func (d *Driver) saveState() {
	logrus.WithField("method", "saveState").Debugf(
		"saving state %#v to '%v'", d.state, d.config.statePath)
	data, err := json.Marshal(d.state)
	if err != nil {
		logrus.WithField("statePath", d.config.statePath).Error(err)
		return
	}

	if err := ioutil.WriteFile(d.config.statePath, data, 0644); err != nil {
		logrus.WithField("savestate", d.config.statePath).Error(err)
	}

}

func (d *Driver) GetOptions() map[string]string {
	return d.config.options
}
func (d *Driver) DedicatesMounts() bool {
	return d.config.dedicatedMounts
}
