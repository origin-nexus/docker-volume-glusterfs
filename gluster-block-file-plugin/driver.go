package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/docker/go-plugins-helpers/volume"
	"github.com/sirupsen/logrus"

	"github.com/origin-nexus/docker-volume-glusterfs/glusterfs-volume"
)

const defaultFileFormat = "%s.img"
const defaultFilesystem = "xfs"

type BlockFileConfig struct {
	filesystem     string
	filenameFormat string
	size           string
}

type GlusterBlockVolume struct {
	glusterfsvolume.MountedVolume
	GlusterVolumeId string
	ImagePath       string
}

func (gbv *GlusterBlockVolume) Mount() error {
	if gbv.IsMounted() {
		return nil
	}

	if err := gbv.CreateMountpoint(); err != nil {
		return fmt.Errorf("error creating mount point: %v)", err)
	}

	if output, err := ExecuteCommand("mount", gbv.ImagePath, gbv.Mountpoint); err != nil {
		return fmt.Errorf("mount command execute failed: %v (%s)", err, output)
	}
	return nil
}

func (gbv *GlusterBlockVolume) createBlockFile(size string, filesystem string) error {
	info, err := os.Stat(gbv.ImagePath)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf("'%v' should be a file, not a dir", gbv.ImagePath)
		}
		return nil
	}

	if size == "" {
		return errors.New("'default-size' option at driver level or 'size' option should be defined")
	}

	output, err := ExecuteCommand("truncate", "-s", size, gbv.ImagePath)
	if err != nil {
		return fmt.Errorf("Image file '%v' creation failed: %v (%s)", gbv.ImagePath, err, output)
	}

	output, err = ExecuteCommand("mkfs."+filesystem, gbv.ImagePath)
	if err != nil {
		return fmt.Errorf("Error creating filsystem '%v': %v (%s)", filesystem, err, output)
	}

	return nil
}

type State struct {
	GlusterBlockVolumes map[string]*GlusterBlockVolume
	GlusterVolumes      glusterfsvolume.State
}

func (state *State) deleteUnused(gvId string) error {
	for _, v := range state.GlusterBlockVolumes {
		if v.GlusterVolumeId == gvId {
			return nil
		}
	}

	gv := state.GlusterVolumes[gvId]
	if err := gv.Unmount(); err != nil {
		return err
	}
	if err := gv.DeleteMountpoint(); err != nil {
		logrus.Warnf("Error deleting Glusterfs mount point: %s", err)
	}

	delete(state.GlusterVolumes, gvId)

	return nil
}

type Driver struct {
	sync.Mutex

	root      string
	statePath string

	glusterConfig   glusterfsvolume.Config
	blockFileConfig BlockFileConfig
	state           State
}

var ExecuteCommand = func(cmd string, args ...string) ([]byte, error) {
	return exec.Command(cmd, args...).CombinedOutput()
}

func (d *Driver) Capabilities() *volume.CapabilitiesResponse {
	logrus.WithField("method", "capabilities").Debugf("")

	return &volume.CapabilitiesResponse{Capabilities: volume.Capability{Scope: "local"}}
}

func (d *Driver) Create(r *volume.CreateRequest) error {
	logrus.WithField("method", "create").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()
	glusterConf := d.glusterConfig
	blockFileConf := d.blockFileConfig

	const optionSetError = "'%v' option already set by driver, can not override."

	for key, val := range r.Options {
		switch key {
		case "servers":
			if glusterConf.Servers != "" {
				return fmt.Errorf(optionSetError, key)
			}
			glusterConf.Servers = val
		case "volume-name":
			if glusterConf.VolumeName != "" {
				return fmt.Errorf(optionSetError, key)
			}
			glusterConf.VolumeName = val
		case "dedicated-mount":
			glusterConf.DedicatedMount = true
		case "filename-format":
			if blockFileConf.filenameFormat != "" {
				return fmt.Errorf(optionSetError, key)
			}
			blockFileConf.filenameFormat = val
		case "filesystem":
			if blockFileConf.filesystem != "" {
				return fmt.Errorf(optionSetError, key)
			}
			blockFileConf.filesystem = val
		case "size":
			blockFileConf.size = val
		default:
			if err := glusterfsvolume.CheckOption(key, val); err != nil {
				return err
			}
			if len(d.glusterConfig.Options) != 0 {
				return errors.New("Gluster options already set by driver, can not override.")
			}
			glusterConf.Options[key] = val
		}
	}

	id, err := d.state.GlusterVolumes.GetOrCreateVolume(glusterConf, filepath.Join(d.root, "gluster-volumes"))
	if err != nil {
		return err
	}

	defer d.saveState()
	defer d.state.deleteUnused(id)

	gv := d.state.GlusterVolumes[id]

	if err := gv.Mount(); err != nil {
		return err
	}

	filename := fmt.Sprintf(blockFileConf.filenameFormat, r.Name)
	if filename == "" {
		filename = fmt.Sprintf(defaultFileFormat, r.Name)
	}

	filesystem := blockFileConf.filesystem
	if filesystem == "" {
		filesystem = defaultFilesystem
	}

	blockVolume := &GlusterBlockVolume{
		GlusterVolumeId: id,
		ImagePath:       filepath.Join(gv.Mountpoint, filename),
		MountedVolume: glusterfsvolume.MountedVolume{
			Mountpoint: filepath.Join(d.root, "block-file-volumes", r.Name)},
	}

	if err := blockVolume.createBlockFile(blockFileConf.size, filesystem); err != nil {
		return fmt.Errorf("Error creating block file: %v", err)
	}

	if err := blockVolume.CreateMountpoint(); err != nil {
		return fmt.Errorf("Error creating mount point: %v", err)
	}

	if err := blockVolume.Mount(); err != nil {
		return fmt.Errorf("Error mounting block file: %v", err)

	}

	d.state.GlusterBlockVolumes[r.Name] = blockVolume

	return nil
}

func (d *Driver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	logrus.WithField("method", "get").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.state.GlusterBlockVolumes[r.Name]
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
	for name, v := range d.state.GlusterBlockVolumes {
		vols = append(vols, &volume.Volume{Name: name, Mountpoint: v.Mountpoint})
	}
	return &volume.ListResponse{Volumes: vols}, nil
}

func (d *Driver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	logrus.WithField("method", "path").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.state.GlusterBlockVolumes[r.Name]
	if !ok {
		return &volume.PathResponse{}, fmt.Errorf("volume %s not found", r.Name)
	}

	return &volume.PathResponse{Mountpoint: v.Mountpoint}, nil
}

func (d *Driver) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	logrus.WithField("method", "mount").Debugf("%#v", r)

	v, ok := d.state.GlusterBlockVolumes[r.Name]
	if !ok {
		return &volume.MountResponse{}, fmt.Errorf("volume %s not found", r.Name)
	}
	logrus.WithField("method", "mount").Debugf("found volume %#v", v)

	if err := d.state.GlusterVolumes[v.GlusterVolumeId].Mount(); err != nil {
		return &volume.MountResponse{}, fmt.Errorf("Error mounting Gluster Volume: %s", err)
	}
	if err := v.Mount(); err != nil {
		return &volume.MountResponse{}, fmt.Errorf("Error mounting Block File: %s", err)
	}

	return &volume.MountResponse{Mountpoint: v.Mountpoint}, nil
}

func (d *Driver) Unmount(r *volume.UnmountRequest) error {
	logrus.WithField("method", "unmount").Debugf("%#v", r)

	if _, ok := d.state.GlusterBlockVolumes[r.Name]; !ok {
		return fmt.Errorf("volume %s not found", r.Name)
	}

	return nil
}

func (d *Driver) Remove(r *volume.RemoveRequest) error {
	logrus.WithField("method", "remove").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.state.GlusterBlockVolumes[r.Name]
	if !ok {
		return fmt.Errorf("volume %s not found", r.Name)
	}

	if err := v.Unmount(); err != nil {
		return fmt.Errorf("Failed to unmount block file: %s", err)
	}

	if err := v.DeleteMountpoint(); err != nil {
		logrus.Warnf("Error deleting block file mount point: %s", err)
	}

	gvId := v.GlusterVolumeId
	delete(d.state.GlusterBlockVolumes, r.Name)

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
