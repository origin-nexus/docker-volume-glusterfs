package main

import (
	"crypto/md5"
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
	glusterVolumeId string
	mountpoint      string
}

type State struct {
	volumes        map[string]Volume
	glusterVolumes map[string]*glusterfsVolume
}

type glusterfsDriver struct {
	sync.Mutex

	root      string
	statePath string

	loglevel string

	servers    string
	volumeName string

	state State
}

func (d *glusterfsDriver) Capabilities() *volume.CapabilitiesResponse {
	logrus.WithField("method", "capabilities").Debugf("")

	return &volume.CapabilitiesResponse{Capabilities: volume.Capability{Scope: "local"}}
}

func (d *glusterfsDriver) Create(r *volume.CreateRequest) error {
	logrus.WithField("method", "create").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()
	gv := &glusterfsVolume{
		servers:    d.servers,
		volumeName: d.volumeName,
		options:    map[string]string{"log-level": d.loglevel},
	}

	for key, val := range r.Options {
		switch key {
		case "backup-volfile-server":
			return errors.New("'backup-volfile-server' option not supported")
		case "backup-volfile-servers":
			return errors.New("'backup-volfile-servers' option not supported")
		case "logfile":
			return errors.New("'logfile' option not supported, logs are redirected to managed plugin stdout")
		case "servers":
			if d.servers != "" {
				return errors.New("'servers' option already set by driver, can not override.")
			}
			gv.servers = val
		case "volume-name":
			if d.volumeName != "" {
				return errors.New("'volume-name' option already set by driver, can not override.")
			}
			gv.volumeName = val
		default:
			gv.options[key] = val
		}
	}

	if gv.servers == "" {
		return errors.New("'servers' option required")
	}

	subdirMount := ""
	if gv.volumeName == "" {
		gv.volumeName = r.Name
	} else {
		subdirMount = r.Name
	}

	id := fmt.Sprintf("%x", md5.Sum([]byte(gv.servers+"/"+gv.volumeName)))
	gv.mountpoint = filepath.Join(d.root, gv.servers, gv.volumeName)
	if existingVolume, ok := d.state.glusterVolumes[id]; ok {
		gv = existingVolume
	}

	mountpoint := gv.mountpoint

	if subdirMount != "" {
		mountpoint = filepath.Join(mountpoint, subdirMount)
	}

	d.state.volumes[r.Name] = Volume{glusterVolumeId: id, mountpoint: mountpoint}
	d.state.glusterVolumes[id] = gv
	d.saveState()

	return nil
}

func (d *glusterfsDriver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	logrus.WithField("method", "get").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.state.volumes[r.Name]
	if !ok {
		return &volume.GetResponse{}, fmt.Errorf("volume %s not found", r.Name)
	}

	return &volume.GetResponse{Volume: &volume.Volume{Name: r.Name, Mountpoint: v.mountpoint}}, nil
}

func (d *glusterfsDriver) List() (*volume.ListResponse, error) {
	logrus.WithField("method", "list").Debugf("")

	d.Lock()
	defer d.Unlock()

	var vols []*volume.Volume
	for name, v := range d.state.volumes {
		vols = append(vols, &volume.Volume{Name: name, Mountpoint: v.mountpoint})
	}
	return &volume.ListResponse{Volumes: vols}, nil
}

func (d *glusterfsDriver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	logrus.WithField("method", "path").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.state.volumes[r.Name]
	if !ok {
		return &volume.PathResponse{}, fmt.Errorf("volume %s not found", r.Name)
	}

	return &volume.PathResponse{Mountpoint: v.mountpoint}, nil
}

func (d *glusterfsDriver) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	logrus.WithField("method", "mount").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.state.volumes[r.Name]
	if !ok {
		return &volume.MountResponse{}, fmt.Errorf("volume %s not found", r.Name)
	}

	gv := d.state.glusterVolumes[v.glusterVolumeId]

	if gv.connections == 0 {
		fi, err := os.Lstat(gv.mountpoint)
		if os.IsNotExist(err) {
			if err := os.MkdirAll(gv.mountpoint, 0755); err != nil {
				return &volume.MountResponse{}, err
			}
		} else if err != nil {
			return &volume.MountResponse{}, err
		}

		if fi != nil && !fi.IsDir() {
			return &volume.MountResponse{},
				fmt.Errorf("%v already exist and it's not a directory", gv.mountpoint)
		}

		if err := gv.mount(); err != nil {
			return &volume.MountResponse{}, err
		}
	}

	// Create subdirectory if required
	if v.mountpoint != gv.mountpoint {
		fi, err := os.Lstat(v.mountpoint)
		if os.IsNotExist(err) {
			if err := os.MkdirAll(v.mountpoint, 0755); err != nil {
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
				fmt.Errorf("%v already exist and it's not a directory", v.mountpoint)
		}
	}

	gv.connections++
	d.saveState()

	return &volume.MountResponse{Mountpoint: v.mountpoint}, nil
}

func (d *glusterfsDriver) Unmount(r *volume.UnmountRequest) error {
	logrus.WithField("method", "unmount").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.state.volumes[r.Name]
	if !ok {
		return fmt.Errorf("volume %s not found", r.Name)
	}

	gv := d.state.glusterVolumes[v.glusterVolumeId]

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

	_, ok := d.state.volumes[r.Name]
	if !ok {
		return fmt.Errorf("volume %s not found", r.Name)
	}

	delete(d.state.volumes, r.Name)
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
	data, err := json.Marshal(d.state.volumes)
	if err != nil {
		logrus.WithField("statePath", d.statePath).Error(err)
		return
	}

	if err := ioutil.WriteFile(d.statePath, data, 0644); err != nil {
		logrus.WithField("savestate", d.statePath).Error(err)
	}
}
