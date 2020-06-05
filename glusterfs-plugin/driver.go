package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/docker/go-plugins-helpers/volume"
	"github.com/sirupsen/logrus"

	"github.com/origin-nexus/docker-volume-glusterfs/glusterfs-volume"
)

type DockerVolume struct {
	GlusterVolumeId string
	Mountpoint      string
}

type State struct {
	DockerVolumes  map[string]*DockerVolume
	GlusterVolumes glusterfsvolume.State
}

type Driver struct {
	sync.Mutex

	root      string
	statePath string

	glusterConfig glusterfsvolume.Config
	state         State
}

func (d *Driver) Capabilities() *volume.CapabilitiesResponse {
	logrus.WithField("method", "capabilities").Debugf("")

	return &volume.CapabilitiesResponse{Capabilities: volume.Capability{Scope: "local"}}
}

func (d *Driver) Create(r *volume.CreateRequest) error {
	logrus.WithField("method", "create").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()
	conf := glusterfsvolume.Config{
		Servers:        d.glusterConfig.Servers,
		VolumeName:     d.glusterConfig.VolumeName,
		Options:        d.glusterConfig.Options,
		DedicatedMount: d.glusterConfig.DedicatedMount,
	}

	for key, val := range r.Options {
		switch key {
		case "servers":
			if conf.Servers != "" {
				return fmt.Errorf("'%v' option already set by driver, can not override.", key)
			}
			conf.Servers = val
		case "volume-name":
			if conf.VolumeName != "" {
				return fmt.Errorf("'%v' option already set by driver, can not override.", key)
			}
			conf.VolumeName = val
		case "dedicated-mount":
			conf.DedicatedMount = true
		default:
			if err := glusterfsvolume.CheckOption(key, val); err != nil {
				return err
			}
			if len(d.glusterConfig.Options) != 0 {
				return errors.New("Options already set by driver, can not override.")
			}
			conf.Options[key] = val
		}
	}

	subdirMount := ""
	if conf.VolumeName == "" {
		conf.VolumeName = r.Name
	} else {
		subdirMount = r.Name
	}

	id, err := d.state.GlusterVolumes.GetOrCreateVolume(conf, d.root)
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
	return d.glusterConfig.Options
}
func (d *Driver) DedicatesMounts() bool {
	return d.glusterConfig.DedicatedMount
}
