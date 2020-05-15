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
)

type Volume struct {
	GlusterVolumeId string
	Mountpoint      string
}

type State struct {
	Volumes        map[string]Volume
	GlusterVolumes map[string]glusterfsVolume
}

type glusterfsDriver struct {
	sync.Mutex

	root      string
	statePath string

	loglevel string

	servers    string
	volumeName string
	options    map[string]string

	state State
}

func (d *glusterfsDriver) Capabilities() *volume.CapabilitiesResponse {
	logrus.WithField("method", "capabilities").Debugf("")

	return &volume.CapabilitiesResponse{Capabilities: volume.Capability{Scope: "local"}}
}

func (d *glusterfsDriver) checkOption(key, val string) error {
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

func (d *glusterfsDriver) Create(r *volume.CreateRequest) error {
	logrus.WithField("method", "create").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()
	gv := glusterfsVolume{
		Servers:    d.servers,
		VolumeName: d.volumeName,
		Options:    d.options,
	}

	for key, val := range r.Options {
		switch key {
		case "servers":
			if d.servers != "" {
				return fmt.Errorf("'%v' option already set by driver, can not override.", key)
			}
			gv.Servers = val
		case "volume-name":
			if d.volumeName != "" {
				return fmt.Errorf("'%v' option already set by driver, can not override.", key)
			}
			gv.VolumeName = val
		default:
			if err := d.checkOption(key, val); err != nil {
				return err
			}
			if len(d.options) != 0 {
				return errors.New("Options already set by driver, can not override.")
			}
			gv.Options[key] = val
		}
	}

	if gv.Servers == "" {
		return errors.New("'servers' option required")
	}

	subdirMount := ""
	if gv.VolumeName == "" {
		gv.VolumeName = r.Name
	} else {
		subdirMount = r.Name
	}

	id := gv.Servers + "/" + gv.VolumeName
	gv.Mountpoint = filepath.Join(d.root, gv.Servers, gv.VolumeName)
	if existingVolume, ok := d.state.GlusterVolumes[id]; ok {
		gv = existingVolume
	}

	mountpoint := gv.Mountpoint

	if subdirMount != "" {
		mountpoint = filepath.Join(mountpoint, subdirMount)
	}

	d.state.Volumes[r.Name] = Volume{GlusterVolumeId: id, Mountpoint: mountpoint}
	d.state.GlusterVolumes[id] = gv
	d.saveState()

	return nil
}

func (d *glusterfsDriver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	logrus.WithField("method", "get").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.state.Volumes[r.Name]
	if !ok {
		return &volume.GetResponse{}, fmt.Errorf("volume %s not found", r.Name)
	}

	return &volume.GetResponse{Volume: &volume.Volume{Name: r.Name, Mountpoint: v.Mountpoint}}, nil
}

func (d *glusterfsDriver) List() (*volume.ListResponse, error) {
	logrus.WithField("method", "list").Debugf("")

	d.Lock()
	defer d.Unlock()

	var vols []*volume.Volume
	for name, v := range d.state.Volumes {
		vols = append(vols, &volume.Volume{Name: name, Mountpoint: v.Mountpoint})
	}
	return &volume.ListResponse{Volumes: vols}, nil
}

func (d *glusterfsDriver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	logrus.WithField("method", "path").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.state.Volumes[r.Name]
	if !ok {
		return &volume.PathResponse{}, fmt.Errorf("volume %s not found", r.Name)
	}

	return &volume.PathResponse{Mountpoint: v.Mountpoint}, nil
}

func (d *glusterfsDriver) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	logrus.WithField("method", "mount").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.state.Volumes[r.Name]
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

func (d *glusterfsDriver) Unmount(r *volume.UnmountRequest) error {
	logrus.WithField("method", "unmount").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.state.Volumes[r.Name]
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

func (d *glusterfsDriver) Remove(r *volume.RemoveRequest) error {
	logrus.WithField("method", "remove").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.state.Volumes[r.Name]
	if !ok {
		return fmt.Errorf("volume %s not found", r.Name)
	}

	gvId := v.GlusterVolumeId
	delete(d.state.Volumes, r.Name)

	gvUsed := false
	for _, v := range d.state.Volumes {
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
func (d *glusterfsDriver) loadState() error {
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

func (d *glusterfsDriver) saveState() {
	data, err := json.Marshal(d.state)
	if err != nil {
		logrus.WithField("statePath", d.statePath).Error(err)
		return
	}

	if err := ioutil.WriteFile(d.statePath, data, 0644); err != nil {
		logrus.WithField("savestate", d.statePath).Error(err)
	}

}
