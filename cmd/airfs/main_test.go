package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sylvanld/airfs"
	"github.com/sylvanld/airfs/sources"
)

// An absent configuration is the first thing a new user hits, so it is answered
// with what to do rather than with the bare open error.
func TestMissingConfigurationSaysHowToCreateIt(t *testing.T) {
	p := paths{target: t.TempDir()}
	p.config = filepath.Join(p.target, sources.FileName)

	_, err := p.load()
	if err == nil || !airfs.IsPrecondition(err) {
		t.Fatalf("err = %v; want a precondition error", err)
	}
	if !strings.Contains(err.Error(), "no source list") {
		t.Errorf("err = %v; should say the list is missing", err)
	}
}

// A missing *source* fails with the same os.ErrNotExist as a missing
// configuration and means something else entirely: reporting it as an absent
// source list sends the reader to a file that is sitting right there.
func TestMissingSourceIsNotReportedAsAMissingConfiguration(t *testing.T) {
	p := paths{target: t.TempDir()}
	p.config = filepath.Join(p.target, sources.FileName)
	if err := os.WriteFile(p.config, []byte("absent-layer\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := p.load()
	if err == nil || !airfs.IsPrecondition(err) {
		t.Fatalf("err = %v; want a precondition error", err)
	}
	if strings.Contains(err.Error(), "no source list") {
		t.Errorf("err = %v; blames the configuration file, which exists", err)
	}
	if !strings.Contains(err.Error(), "absent-layer") {
		t.Errorf("err = %v; should name the source that is missing", err)
	}
}
