package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/pkg/mount"
	"github.com/docker/docker/pkg/parsers"
	"github.com/pkg/errors"

	"github.com/docker/go-plugins-helpers/volume"
	"golang.org/x/sys/unix"
)

const (
	driverName = "tmpsync"
	configFile = "tmpsync.json"
)

type tmpsyncVolume struct {
	Mountpoint string `json:"mountpoint"`
	FsSize     string `json:"fssize"`
	Target     string `json:"target"`
	OpMode     string `json:"opmode"`
	SshKey     string `json:"sshkey"`
}

type tmpsyncOptions struct {
	RootPath string
}

type tmpsyncDriver struct {
	options tmpsyncOptions

	sync.RWMutex

	Volumes map[string]*tmpsyncVolume `json:"volumes"`
}

func parseOptions(options []string) (*tmpsyncOptions, error) {
	opts := &tmpsyncOptions{}
	for _, opt := range options {
		key, val, err := parsers.ParseKeyValueOpt(opt)
		if err != nil {
			return nil, err
		}
		key = strings.ToLower(key)
		switch key {
		case "root":
			opts.RootPath, _ = filepath.Abs(val)
		default:
			return nil, errors.Errorf("tmpsync: unknown option (%s = %s)", key, val)
		}
	}

	return opts, nil
}

func (d *tmpsyncDriver) getMntPath(name string) string {
	return path.Join(d.options.RootPath, name)
}

func (d *tmpsyncDriver) getConfigPath() string {
	return path.Join(d.options.RootPath, configFile)
}

func (d *tmpsyncDriver) syncVolume(v *tmpsyncVolume) error {
	args := []string{}

	if strings.Contains(v.OpMode, "archive") {
		args = append(args, "--archive")
	}
	if strings.Contains(v.OpMode, "compress") {
		args = append(args, "--compress")
	}
	if strings.Contains(v.OpMode, "delete") {
		args = append(args, "--delete")
		args = append(args, "--recursive")
	} else if strings.Contains(v.OpMode, "recursive") {
		args = append(args, "--recursive")
	}
	if v.SshKey != "" {
		args = append(args, "-e")
		args = append(args, fmt.Sprintf("ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=quiet -i %v", v.SshKey))
	}

	args = append(args, fmt.Sprintf("%v/", v.Mountpoint))
	args = append(args, v.Target)

	if out, err := exec.Command("rsync", args...).CombinedOutput(); err != nil {
		log.Println(string(out))
		return err
	}

	return nil
}

func (d *tmpsyncDriver) Load() error {
	jsonpath := d.getConfigPath()

	if jsondata, err := ioutil.ReadFile(jsonpath); err == nil {
		if err := json.Unmarshal(jsondata, &d); err != nil {
			return errors.New("could not parse json config")
		}

		return nil
	}

	return nil
}

func (d *tmpsyncDriver) Flush() error {
	jsonpath := d.getConfigPath()

	jsondata, err := json.Marshal(d)
	if err != nil {
		return errors.New("could not encode json config")
	}

	tmpfile, err := ioutil.TempFile(filepath.Dir(jsonpath), ".tmp")
	if err != nil {
		return errors.New("could not create temp file for json config")
	}

	n, err := tmpfile.Write(jsondata)
	if err != nil {
		return errors.New("could not write json config to temp file")
	}
	if n < len(jsondata) {
		return io.ErrShortWrite
	}
	if err := tmpfile.Sync(); err != nil {
		return errors.New("could not sync temp file")
	}
	if err := tmpfile.Close(); err != nil {
		return errors.New("could not close temp file")
	}
	if err := os.Rename(tmpfile.Name(), jsonpath); err != nil {
		return errors.New("could not commit json config")
	}

	return nil
}

func (d *tmpsyncDriver) Create(r *volume.CreateRequest) error {
	log.Printf("tmpsync: create (%v)\n", r)

	d.Lock()
	defer d.Unlock()

	v := &tmpsyncVolume{}
	v.Mountpoint = d.getMntPath(r.Name)

	if err := os.MkdirAll(v.Mountpoint, 0755); err != nil {
		return err
	}

	for key, val := range r.Options {
		switch key {
		case "fssize":
			v.FsSize = val
		case "target":
			v.Target = val
		case "opmode":
			v.OpMode = val
		case "sshkey":
			v.SshKey = val
		default:
			return errors.Errorf("tmpsync: unknown option (%s = %s)", key, val)
		}
	}

	d.Volumes[r.Name] = v

	d.Flush()

	return nil
}

func (d *tmpsyncDriver) Remove(r *volume.RemoveRequest) error {
	log.Printf("tmpsync: remove (%v)\n", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.Volumes[r.Name]
	if !ok {
		return errors.Errorf("tmpsync: volume %s not found", r.Name)
	}

	if err := os.RemoveAll(v.Mountpoint); err != nil {
		return err
	}

	delete(d.Volumes, r.Name)

	d.Flush()

	return nil
}

func (d *tmpsyncDriver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	log.Printf("tmpsync: path (%v)\n", r)

	d.RLock()
	defer d.RUnlock()

	v, ok := d.Volumes[r.Name]
	if !ok {
		return &volume.PathResponse{}, errors.Errorf("tmpsync: volume %s not found", r.Name)
	}

	return &volume.PathResponse{Mountpoint: v.Mountpoint}, nil
}

func (d *tmpsyncDriver) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	log.Printf("tmpsync: mount (%v)\n", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.Volumes[r.Name]
	if !ok {
		return &volume.MountResponse{}, errors.Errorf("tmpsync: volume %s not found", r.Name)
	}

	if err := unix.Mount("tmpfs", v.Mountpoint, "tmpfs", 0, fmt.Sprintf("size=%v", v.FsSize)); err != nil {
		return &volume.MountResponse{}, errors.Errorf("tmpsync: could not mount tmpfs on %v", r.Name)
	}

	return &volume.MountResponse{
		Mountpoint: v.Mountpoint,
	}, nil
}

func (d *tmpsyncDriver) Unmount(r *volume.UnmountRequest) error {
	log.Printf("tmpsync: unmount (%v)\n", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.Volumes[r.Name]
	if !ok {
		return errors.Errorf("tmpsync: volume %s not found", r.Name)
	}

	if err := d.syncVolume(v); err != nil {
		return errors.Errorf("tmpsync: could not sync volume on %v", r.Name)
	}

	mount.RecursiveUnmount(v.Mountpoint)

	return nil
}

func (d *tmpsyncDriver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	log.Printf("tmpsync: get (%v)\n", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.Volumes[r.Name]
	if !ok {
		return &volume.GetResponse{}, errors.Errorf("tmpsync: volume %s not found", r.Name)
	}

	return &volume.GetResponse{
		Volume: &volume.Volume{
			Name:       r.Name,
			Mountpoint: v.Mountpoint,
			CreatedAt:  time.Now().Format(time.RFC3339),
		},
	}, nil
}

func (d *tmpsyncDriver) List() (*volume.ListResponse, error) {
	log.Printf("tmpsync: list ()\n")

	d.Lock()
	defer d.Unlock()

	var volumes []*volume.Volume
	for name, v := range d.Volumes {
		volumes = append(volumes, &volume.Volume{Name: name, Mountpoint: v.Mountpoint})
	}

	return &volume.ListResponse{
		Volumes: volumes,
	}, nil
}

func (d *tmpsyncDriver) Capabilities() *volume.CapabilitiesResponse {
	log.Printf("tmpsync: capabilities ()\n")

	return &volume.CapabilitiesResponse{
		Capabilities: volume.Capability{
			Scope: "local",
		},
	}
}

func NewTmpsyncDriver(options []string) (*tmpsyncDriver, error) {
	log.Printf("tmpsync: createDriver (%v)\n", options)

	opts, err := parseOptions(options)
	if err != nil {
		return nil, err
	}

	d := &tmpsyncDriver{
		Volumes: map[string]*tmpsyncVolume{},
	}
	d.options = *opts

	d.Load()

	return d, nil
}
