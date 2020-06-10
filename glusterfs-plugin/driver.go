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
	glusterfsvolume.MountedVolume
	GlusterVolumeId string
}

type State struct {
	DockerVolumes  map[string]*DockerVolume
	GlusterVolumes glusterfsvolume.State
}

func (s *State) deleteUnused(gvId string) error {
	for _, v := range s.DockerVolumes {
		if v.GlusterVolumeId == gvId {
			return nil
		}
	}

	gv := s.GlusterVolumes[gvId]
	if err := gv.Unmount(); err != nil {
		return err
	}
	if err := gv.DeleteMountpoint(); err != nil {
		logrus.Warnf("Error deleting block file mount point: %s", err)
	}

	delete(s.GlusterVolumes, gvId)

	return nil
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
	conf := d.glusterConfig.Copy()

	const optionSetError = "'%v' option already set by driver, can not override."

	for key, val := range r.Options {
		switch key {
		case "servers":
			if conf.Servers != "" {
				return fmt.Errorf(optionSetError, key)
			}
			conf.Servers = val
		case "volume-name":
			if conf.VolumeName != "" {
				return fmt.Errorf(optionSetError, key)
			}
			conf.VolumeName = val
		case "dedicated-mount":
			conf.DedicatedMount = true
		default:
			if err := glusterfsvolume.CheckOption(key, val); err != nil {
				return err
			}
			if len(d.glusterConfig.Options) != 0 {
				return errors.New("Gluster options already set by driver, can not override.")
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
	defer d.state.deleteUnused(id)

	gv := d.state.GlusterVolumes[id]

	if err := gv.Mount(); err != nil {
		return err
	}

	dockerVolume := &DockerVolume{
		GlusterVolumeId: id,
		MountedVolume:   glusterfsvolume.MountedVolume{Mountpoint: gv.Mountpoint},
	}
	if subdirMount != "" {
		dockerVolume.Mountpoint = filepath.Join(dockerVolume.Mountpoint, subdirMount)
		if err := dockerVolume.CreateMountpoint(); err != nil {
			return err
		}
	}

	d.state.DockerVolumes[r.Name] = dockerVolume

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

	if err := d.state.GlusterVolumes[v.GlusterVolumeId].Mount(); err != nil {
		return &volume.MountResponse{}, fmt.Errorf("Error mounting Gluster Volume: %s", err)
	}
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

	if err := d.state.deleteUnused(gvId); err != nil {
		return err
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
	return d.glusterConfig.Options
}
func (d *Driver) DedicatesMounts() bool {
	return d.glusterConfig.DedicatedMount
}
