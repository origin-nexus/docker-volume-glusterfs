package glusterfsvolume

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
)

type State map[string]*Volume

type Config struct {
	DedicatedMount bool

	Servers    string
	VolumeName string
	Options    map[string]string
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

func (s State) GetOrCreateVolume(config Config, root string) (string, error) {
	// returns an ID of a Volume, creates the volume if needed.
	gv := &Volume{
		Servers:    config.Servers,
		VolumeName: config.VolumeName,
	}

	for key, val := range config.Options {
		if err := CheckOption(key, val); err != nil {
			return "", err
		}
		gv.Options[key] = val
	}

	if gv.Servers == "" {
		return "", errors.New("'servers' option required")
	}
	if gv.VolumeName == "" {
		return "", errors.New("'volume-name' option required")
	}

	id := ""
	if config.DedicatedMount {
		i := 1
		for {
			id = filepath.Join("_dedicated", gv.Servers, gv.VolumeName, string(i))
			if _, exists := s[id]; !exists {
				break
			}
			i++
		}
	} else {
		id = filepath.Join(gv.Servers, gv.VolumeName)
	}
	gv.Mountpoint = filepath.Join(root, id)
	if existingVolume, ok := s[id]; ok {
		if !reflect.DeepEqual(gv.Options, existingVolume.Options) {
			return "", fmt.Errorf(
				"%#v options differ from already created volumes with options %#v"+
					" use 'dedicated-mount' option to not reuse existing mounts.",
				gv.Options, existingVolume.Options)
		}
		gv = existingVolume
	} else {
		s[id] = gv
	}

	return id, nil
}
