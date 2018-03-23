package basic

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"sync"

	"github.com/sapk/docker-volume-helpers/driver"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"github.com/docker/go-plugins-helpers/volume"
)

type Mountpoint struct {
	Path        string `json:"path"`
	Connections int    `json:"connections"`
}

func (d *Mountpoint) GetPath() string {
	return d.Path
}

func (d *Mountpoint) GetConnections() int {
	return d.Connections
}

func (d *Mountpoint) SetConnections(n int) {
	d.Connections = n
}

type Volume struct {
	VolumeURI   string `json:"voluri"`
	Mount       string `json:"mount"`
	Connections int    `json:"connections"`
}

func (v *Volume) GetMount() string {
	return v.Mount
}

func (v *Volume) GetRemote() string {
	return v.VolumeURI
}

func (v *Volume) GetConnections() int {
	return v.Connections
}

func (v *Volume) SetConnections(n int) {
	v.Connections = n
}

func (v *Volume) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"TODO": "List",
	}
}

//Driver the global driver responding to call
type Driver struct {
	lock          sync.RWMutex
	root          string
	mountUniqName bool
	persistence   *viper.Viper
	volumes       map[string]*Volume
	mounts        map[string]*Mountpoint
	cfgfolder     string
	version       int
}

func (d *Driver) GetVolumes() map[string]driver.Volume {
	vi := make(map[string]driver.Volume, len(d.volumes))
	for k, i := range d.volumes {
		vi[k] = i
	}
	return vi
}

func (d *Driver) GetMounts() map[string]driver.Mount {
	mi := make(map[string]driver.Mount, len(d.mounts))
	for k, i := range d.mounts {
		mi[k] = i
	}
	return mi
}

func (d *Driver) GetLock() *sync.RWMutex {
	return &d.lock
}

//List volumes handled by these driver
func (d *Driver) List() (*volume.ListResponse, error) {
	return driver.List(d)
}

//Get get info on the requested volume
func (d *Driver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	v, m, err := driver.Get(d, r.Name)
	if err != nil {
		return nil, err
	}
	return &volume.GetResponse{Volume: &volume.Volume{Name: r.Name, Status: v.GetStatus(), Mountpoint: m.GetPath()}}, nil
}

//Remove remove the requested volume
func (d *Driver) Remove(r *volume.RemoveRequest) error {
	return driver.Remove(d, r.Name)
}

//Unmount unmount the requested volume
func (d *Driver) Unmount(r *volume.UnmountRequest) error {
	return driver.Unmount(d, r.Name)
}

//Capabilities Send capabilities of the local driver
func (d *Driver) Capabilities() *volume.CapabilitiesResponse {
	return driver.Capabilities()
}

//RunCmd run deamon in context of this gvfs drive with custome env
func (d *Driver) RunCmd(cmd string) error {
	logrus.Debugf(cmd)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		logrus.Debugf("Error: %v", err)
	}
	logrus.Debugf("Output: %v", out)
	return err
}

//Persistence represent struct of persistence file
type Persistence struct {
	Version int                    `json:"version"`
	Volumes map[string]*Volume     `json:"volumes"`
	Mounts  map[string]*Mountpoint `json:"mounts"`
}

//SaveConfig stroe config/state in file
func (d *Driver) SaveConfig() error {
	fi, err := os.Lstat(d.cfgfolder)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(d.cfgfolder, 0700); err != nil {
			return fmt.Errorf("SaveConfig: %s", err)
		}
	} else if err != nil {
		return fmt.Errorf("SaveConfig: %s", err)
	}
	if fi != nil && !fi.IsDir() {
		return fmt.Errorf("SaveConfig: %v already exist and it's not a directory", d.root)
	}
	b, err := json.Marshal(Persistence{Version: d.version, Volumes: d.volumes, Mounts: d.mounts})
	if err != nil {
		logrus.Warn("Unable to encode persistence struct, %v", err)
	}
	//logrus.Debug("Writing persistence struct, %v", b, d.volumes)
	err = ioutil.WriteFile(d.cfgfolder+"/persistence.json", b, 0600)
	if err != nil {
		logrus.Warn("Unable to write persistence struct, %v", err)
		return fmt.Errorf("SaveConfig: %s", err)
	}
	return nil
}

//Path get path of the requested volume
func (d *Driver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	_, m, err := driver.Get(d, r.Name)
	if err != nil {
		return nil, err
	}
	return &volume.PathResponse{Mountpoint: m.GetPath()}, nil
}
