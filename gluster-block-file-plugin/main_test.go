package main

import (
	"os"
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestNewDriverUnknownLoglevel(t *testing.T) {
	root := "/myroot"
	os.Setenv("LOGLEVEL", "UNKONW")
	_, err := NewDriver(root)

	if err == nil {
		t.Error("Unknown LOGLEVEL should return error")
	}
}
func TestNewDriverDefaultLoglevel(t *testing.T) {
	root := "/myroot"
	os.Setenv("LOGLEVEL", "")
	_, err := NewDriver(root)

	if err != nil {
		t.Error("LOGLEVEL '' should not return error")
		return
	}

	if l := logrus.GetLevel(); l != logrus.WarnLevel {
		t.Errorf("LOGLEVEL '' should set logrus level to '%v', not '%v'", logrus.WarnLevel, l)
	}
}

func TestNewDriverLogruslevel(t *testing.T) {
	cases := []struct {
		envLevel    string
		logrusLevel logrus.Level
	}{
		{"TRACE", logrus.TraceLevel},
		{"DEBUG", logrus.DebugLevel},
		{"INFO", logrus.InfoLevel},
		{"WARNING", logrus.WarnLevel},
		{"ERROR", logrus.ErrorLevel},
		{"CRITICAL", logrus.ErrorLevel},
		{"NONE", logrus.ErrorLevel},
	}

	root := "/myroot"

	for _, c := range cases {
		os.Setenv("LOGLEVEL", c.envLevel)
		_, err := NewDriver(root)

		if err != nil {
			t.Errorf("LOGLEVEL '%v' should not return error", c.envLevel)
			return
		}

		if l := logrus.GetLevel(); l != c.logrusLevel {
			t.Errorf("LOGLEVEL '%v' should set logrus level to '%v', not '%v'", c.envLevel, c.logrusLevel, l)
		}
	}
}

func TestUnsupportedOptionsInMain(t *testing.T) {
	unsupportedOptions := []string{
		"backup-volfile-server", "backup-volfile-servers", "log-file", "servers",
		"volume-name", "log-level=ERROR log-file=/whatever"}
	root := "/myroot"

	for _, option := range unsupportedOptions {
		os.Setenv("OPTIONS", option)
		_, err := NewDriver(root)

		if err == nil {
			t.Errorf("Unsupported option '%v' should return error", option)
		}
	}
}

func TestOPTIONvarSetsOptions(t *testing.T) {
	option_str := "acl log-level=INFO"
	os.Setenv("OPTIONS", option_str)
	root := "/myroot"
	d, err := NewDriver(root)

	if err != nil {
		t.Error("Correct options should not raise error")
	}
	options := d.GetOptions()
	if !reflect.DeepEqual(options, map[string]string{
		"acl":       "",
		"log-level": "INFO",
	}) {
		t.Errorf(
			"Driver options not set correctly from env var OPTIONS='%v': %#v",
			option_str, options)
	}
	if d.DedicatesMounts() {
		t.Error("Dedicated mounts per docker volume when it was not set in options")
	}
}

func TestOPTIONdedicatedMount(t *testing.T) {
	option_str := "acl log-level=INFO dedicated-mount"
	os.Setenv("OPTIONS", option_str)
	root := "/myroot"
	d, err := NewDriver(root)

	if err != nil {
		t.Error("Correct options should not raise error")
	}
	options := d.GetOptions()
	if !reflect.DeepEqual(options, map[string]string{
		"acl":       "",
		"log-level": "INFO",
	}) {
		t.Errorf(
			"Driver options not set correctly from env var OPTIONS='%v': %#v",
			option_str, options)
	}
	if !d.DedicatesMounts() {
		t.Error(
			"Dedicated mounts was not activated by 'dedicated-mounts' option")
	}
}
