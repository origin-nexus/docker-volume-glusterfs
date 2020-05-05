package main

import (
	"os"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestNewGlusterfsDriverUnknownLoglevel(t *testing.T) {
	root := "/myroot"
	os.Setenv("LOGLEVEL", "UNKONW")
	_, err := newGlusterfsDriver(root)

	if err == nil {
		t.Error("Unknown LOGLEVEL should return error")
	}
}
func TestNewGlusterfsDriverDefaultLoglevel(t *testing.T) {
	root := "/myroot"
	os.Setenv("LOGLEVEL", "")
	d, err := newGlusterfsDriver(root)

	if err != nil {
		t.Error("LOGLEVEL '' should not return error")
		return
	}

	if d.loglevel != "WARNING" {
		t.Error("LOGLEVEL '' should set loglevel to 'WARNING'")
	}

	if l := logrus.GetLevel(); l != logrus.WarnLevel {
		t.Errorf("LOGLEVEL '' should set logrus level to '%v', not '%v'", logrus.WarnLevel, l)
	}
}

func TestNewGlusterfsDriverLogruslevel(t *testing.T) {
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
		d, err := newGlusterfsDriver(root)

		if err != nil {
			t.Errorf("LOGLEVEL '%v' should not return error", c.envLevel)
			return
		}

		if l := logrus.GetLevel(); l != c.logrusLevel {
			t.Errorf("LOGLEVEL '%v' should set logrus level to '%v', not '%v'", c.envLevel, c.logrusLevel, l)
		}

		if d.loglevel != c.envLevel {
			t.Errorf("LOGLEVEL '%v' should set logLevel to '%v'", c.envLevel, c.envLevel)
		}
	}
}
