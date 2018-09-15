package main

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/pkg/parsers"
	"github.com/docker/go-units"
	"github.com/pkg/errors"

	"github.com/docker/go-plugins-helpers/volume"
)

const (
	driverName = "tmpsync"
)

type tmpsyncVolume struct {
	Options []string

	Mountpoint string
	FsSize     uint64
}

type tmpsyncOptions struct {
}

type tmpsyncDriver struct {
	options tmpsyncOptions

	sync.RWMutex

	volumes map[string]*tmpsyncVolume
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
		default:
			return nil, errors.Errorf("tmpsync: unknown option (%s = %s)", key, val)
		}
	}

	return opts, nil
}

func (d *tmpsyncDriver) Create(r *volume.CreateRequest) error {
	log.Printf("tmpsync: create (%v)\n", r)

	d.Lock()
	defer d.Unlock()

	v := &tmpsyncVolume{}

	for key, val := range r.Options {
		switch key {
		case "path":
			v.Mountpoint = val
		case "size":
			size, _ := units.RAMInBytes(val)
			v.FsSize = uint64(size)
		default:
			return errors.Errorf("tmpsync: unknown option (%s = %s)", key, val)
		}
	}

	d.volumes[r.Name] = v

	return nil
}

func (d *tmpsyncDriver) Remove(r *volume.RemoveRequest) error {
	log.Printf("tmpsync: remove (%v)\n", r)

	d.Lock()
	defer d.Unlock()

	delete(d.volumes, r.Name)

	return nil
}

func (d *tmpsyncDriver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	log.Printf("tmpsync: path (%v)\n", r)

	d.RLock()
	defer d.RUnlock()

	v, ok := d.volumes[r.Name]
	if !ok {
		return &volume.PathResponse{}, errors.Errorf("tmpsync: volume %s not found", r.Name)
	}

	return &volume.PathResponse{Mountpoint: v.Mountpoint}, nil
}

func (d *tmpsyncDriver) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	log.Printf("tmpsync: mount (%v)\n", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.volumes[r.Name]
	if !ok {
		return &volume.MountResponse{}, errors.Errorf("tmpsync: volume %s not found", r.Name)
	}

	return &volume.MountResponse{
		Mountpoint: v.Mountpoint,
	}, nil
}

func (d *tmpsyncDriver) Unmount(r *volume.UnmountRequest) error {
	log.Printf("tmpsync: unmount (%v)\n", r)

	d.Lock()
	defer d.Unlock()

	return nil
}

func (d *tmpsyncDriver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	log.Printf("tmpsync: get (%v)\n", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.volumes[r.Name]
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
	for name, v := range d.volumes {
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
		volumes: map[string]*tmpsyncVolume{},
	}
	d.options = *opts

	return d, nil
}
