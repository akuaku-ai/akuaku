package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// namesFile is the monitor-owned overlay of custom display names. Keeping renames
// here rather than in the run files means producers still own their run state and
// a rename never races a producer's write.
const namesFile = "names.json"

// ReadNames returns the custom-name overlay: run id -> display name. Names are
// optional metadata, so a missing or unparseable overlay yields an empty map.
func ReadNames(dir string) map[string]string {
	data, err := os.ReadFile(filepath.Join(dir, namesFile))
	if err != nil {
		return map[string]string{}
	}
	names := map[string]string{}
	if err := json.Unmarshal(data, &names); err != nil {
		return map[string]string{}
	}
	return names
}

// WriteName records a custom display name for id, preserving other entries. The
// write is atomic, so a crash never corrupts the overlay.
func WriteName(dir, id, name string) error {
	names := ReadNames(dir)
	names[id] = name

	data, err := marshal(names)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}
	final := filepath.Join(dir, namesFile)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, filePerm); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}
