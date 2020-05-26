package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/go-plugins-helpers/volume"
	"github.com/sirupsen/logrus"

	"./glusterfs-driver"
)

const socketAddress = "/run/docker/plugins/glusterfs.sock"

func newGlusterfsDriver(root string) (*glusterfsdriver.Driver, error) {
	logrus.WithField("method", "new glusterfs driver").Debug(root)

	dedicatedMounts := false
	options := map[string]string{}

	options_str := os.Getenv("OPTIONS")
	for _, option := range strings.Split(options_str, " ") {
		if option == "" {
			continue
		}
		kv := strings.SplitN(option, "=", 2)
		if len(kv) == 1 {
			kv = append(kv, "")
		}
		switch kv[0] {
		case "dedicated-mount":
			dedicatedMounts = true
		default:
			if err := glusterfsdriver.CheckOption(kv[0], kv[1]); err != nil {
				return nil, err
			}
			options[kv[0]] = kv[1]
		}
	}

	loglevel := os.Getenv("LOGLEVEL")
	switch loglevel {
	case "TRACE":
		logrus.SetLevel(logrus.TraceLevel)
	case "DEBUG":
		logrus.SetLevel(logrus.DebugLevel)
	case "INFO":
		logrus.SetLevel(logrus.InfoLevel)
	case "":
		loglevel = "WARNING"
		fallthrough
	case "WARNING":
		logrus.SetLevel(logrus.WarnLevel)
	case "ERROR":
		logrus.SetLevel(logrus.ErrorLevel)
	case "CRITICAL":
		logrus.SetLevel(logrus.ErrorLevel)
	case "NONE":
		logrus.SetLevel(logrus.ErrorLevel)
	default:
		return nil, fmt.Errorf("unknown log level '%v'", loglevel)
	}

	d := glusterfsdriver.NewDriver(
		root, filepath.Join(root, "glusterfs-state.json"),
		os.Getenv("SERVERS"), os.Getenv("VOLUME_NAME"), dedicatedMounts, options)

	if err := d.LoadState(); err != nil {
		logrus.Error(err)
	}

	return d, nil
}

func main() {
	d, err := newGlusterfsDriver("/mnt")
	if err != nil {
		logrus.Fatal(err)
	}
	h := volume.NewHandler(d)
	logrus.Infof("listening on %s", socketAddress)
	logrus.Error(h.ServeUnix(socketAddress, 0))
}
