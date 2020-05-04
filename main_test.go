package main

import (
	"os"
	"testing"
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
}
