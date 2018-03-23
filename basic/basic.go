package basic

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	Lock          sync.RWMutex
	Root          string
	MountUniqName bool
	Persistence   *viper.Viper
	Volumes       map[string]*Volume
	Mounts        map[string]*Mountpoint
	CfgFolder     string
	Version       int
	IsValidURI    func(string) bool
	MountVolume   func(*Driver, driver.Volume, driver.Mount, *volume.MountRequest) (*volume.MountResponse, error)
}

func (d *Driver) GetVolumes() map[string]driver.Volume {
	vi := make(map[string]driver.Volume, len(d.Volumes))
	for k, i := range d.Volumes {
		vi[k] = i
	}
	return vi
}

func (d *Driver) GetMounts() map[string]driver.Mount {
	mi := make(map[string]driver.Mount, len(d.Mounts))
	for k, i := range d.Mounts {
		mi[k] = i
	}
	return mi
}

func (d *Driver) GetLock() *sync.RWMutex {
	return &d.Lock
}

//Create create and init the requested volume
func (d *Driver) Create(r *volume.CreateRequest) error {
	logrus.Debugf("Entering Create: name: %s, options %v", r.Name, r.Options)

	if r.Options == nil || r.Options["voluri"] == "" {
		return fmt.Errorf("voluri option required")
	}
	r.Options["voluri"] = strings.Trim(r.Options["voluri"], "\"")
	if !d.IsValidURI(r.Options["voluri"]) {
		return fmt.Errorf("voluri option is malformated")
	}

	d.GetLock().Lock()
	defer d.GetLock().Unlock()

	v := &Volume{
		VolumeURI:   r.Options["voluri"],
		Mount:       GetMountName(d, r),
		Connections: 0,
	}

	if _, ok := d.Mounts[v.Mount]; !ok { //This mountpoint doesn't allready exist -> create it
		m := &Mountpoint{
			Path:        filepath.Join(d.Root, v.Mount),
			Connections: 0,
		}

		_, err := os.Lstat(m.Path) //Create folder if not exist. This will also failed if already exist
		if os.IsNotExist(err) {
			if err = os.MkdirAll(m.Path, 0700); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		isempty, err := FolderIsEmpty(m.Path)
		if err != nil {
			return err
		}
		if !isempty {
			return fmt.Errorf("%v already exist and is not empty", m.Path)
		}
		d.Mounts[v.Mount] = m
	}

	d.Volumes[r.Name] = v
	logrus.Debugf("Volume Created: %v", v)
	return d.SaveConfig()
}

//List Volumes handled by these driver
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
		logrus.Warnf("Error: %s", err)
	}
	logrus.Warnf("Output: %s", out)
	return err
}

//Persistence represent struct of Persistence file
type Persistence struct {
	Version int                    `json:"version"`
	Volumes map[string]*Volume     `json:"volumes"`
	Mounts  map[string]*Mountpoint `json:"mounts"`
}

//SaveConfig stroe config/state in file
func (d *Driver) SaveConfig() error {
	fi, err := os.Lstat(d.CfgFolder)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(d.CfgFolder, 0700); err != nil {
			return fmt.Errorf("SaveConfig: %s", err)
		}
	} else if err != nil {
		return fmt.Errorf("SaveConfig: %s", err)
	}
	if fi != nil && !fi.IsDir() {
		return fmt.Errorf("SaveConfig: %v already exist and it's not a directory", d.Root)
	}
	b, err := json.Marshal(Persistence{Version: d.Version, Volumes: d.Volumes, Mounts: d.Mounts})
	if err != nil {
		logrus.Warn("Unable to encode Persistence struct, %v", err)
	}
	//logrus.Debug("Writing Persistence struct, %v", b, d.Volumes)
	err = ioutil.WriteFile(d.CfgFolder+"/persistence.json", b, 0600)
	if err != nil {
		logrus.Warn("Unable to write Persistence struct, %v", err)
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

//Mount mount the requested volume
func (d *Driver) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	logrus.Debugf("Entering Mount: %v", r)

	v, m, err := driver.MountExist(d, r.Name)
	if err != nil {
		return nil, err
	}
	if m != nil && m.GetConnections() > 0 {
		return &volume.MountResponse{Mountpoint: m.GetPath()}, nil
	}

	d.GetLock().Lock()
	defer d.GetLock().Unlock()

	resp, err := d.MountVolume(d, v, m, r)
	if err != nil {
		return nil, err
	}
	//time.Sleep(3 * time.Second)
	driver.AddN(1, v, m)
	return resp, d.SaveConfig()
}
